package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/server/internal/contract"
	serverbudget "github.com/kave-io/kave/server/ops/budget"
	serverpolicy "github.com/kave-io/kave/server/ops/policy"
)

type blockInterceptor struct {
	err error
}

func (b blockInterceptor) Before(_ context.Context, _ *runtime.Action) (*runtime.Action, error) {
	return nil, b.err
}

func (b blockInterceptor) After(context.Context, *runtime.Action, *pipeline.Result) error { return nil }
func (b blockInterceptor) Name() string                                                   { return "block" }

func TestGatewayRoutes(t *testing.T) {
	t.Setenv("KAVE_TEST_PROVIDER_KEY", "real-key")

	baseApp := &mockAppStore{
		agent: &controlmodel.Agent{ID: "a1", ProjectID: "proj1", EnvID: "default", Status: controlmodel.AgentStatusActive},
		token: &controlmodel.AgentToken{ID: "tok1", AgentID: "a1", OrgID: "default", TokenHash: []byte("ignored")},
		cred:  &controlmodel.ConnectorCredential{Source: controlmodel.CredentialSourceEnv, EnvVar: "KAVE_TEST_PROVIDER_KEY", ProjectID: "proj1"},
	}

	tests := []struct {
		name           string
		path           string
		body           []byte
		expectPath     string
		expectStatus   int
		expectCode     string
		pipeline       *pipeline.Pipeline
		expectUpstream bool
		upstreamStatus int
		upstreamBody   []byte
	}{
		{
			name:           "raw_openai",
			path:           "/v1/openai/chat/completions",
			body:           []byte(`{"model":"gpt-4o","messages":[]}`),
			expectPath:     "/v1/chat/completions",
			expectStatus:   http.StatusOK,
			pipeline:       pipeline.New(),
			expectUpstream: true,
			upstreamStatus: http.StatusOK,
			upstreamBody:   []byte(`{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":20}}`),
		},
		{
			name:           "raw_anthropic",
			path:           "/v1/anthropic/messages",
			body:           []byte(`{"model":"claude-3-5-sonnet","messages":[]}`),
			expectPath:     "/v1/messages",
			expectStatus:   http.StatusOK,
			pipeline:       pipeline.New(),
			expectUpstream: true,
			upstreamStatus: http.StatusOK,
			upstreamBody:   []byte(`{"model":"claude-3-5-sonnet","usage":{"input_tokens":7,"output_tokens":11}}`),
		},
		{
			name:           "raw_google",
			path:           "/v1/google/v1beta/models/gemini-1.5-flash:generateContent",
			body:           []byte(`{"contents":[]}`),
			expectPath:     "/v1beta/models/gemini-1.5-flash:generateContent",
			expectStatus:   http.StatusOK,
			pipeline:       pipeline.New(),
			expectUpstream: true,
			upstreamStatus: http.StatusOK,
			upstreamBody:   []byte(`{"modelVersion":"gemini-1.5-flash","usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":8}}`),
		},
		{
			name:           "framework_claude_code",
			path:           "/frameworks/claude-code/openai/v1/chat/completions",
			body:           []byte(`{"model":"gpt-4o","messages":[]}`),
			expectPath:     "/v1/chat/completions",
			expectStatus:   http.StatusOK,
			pipeline:       pipeline.New(),
			expectUpstream: true,
			upstreamStatus: http.StatusOK,
			upstreamBody:   []byte(`{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":20}}`),
		},
		{
			name:           "policy_blocked",
			path:           "/v1/openai/chat/completions",
			body:           []byte(`{"model":"gpt-4o","messages":[]}`),
			expectStatus:   http.StatusForbidden,
			expectCode:     "gateway.policy_blocked",
			pipeline:       pipeline.New(blockInterceptor{err: &serverpolicy.BlockedError{Reason: "policy denied", Subject: "a1", Object: "openai.chat.completions"}}),
			expectUpstream: false,
		},
		{
			name:           "budget_blocked",
			path:           "/frameworks/claude-code/openai/v1/chat/completions",
			body:           []byte(`{"model":"gpt-4o","messages":[]}`),
			expectStatus:   http.StatusPaymentRequired,
			expectCode:     "gateway.budget_exceeded",
			pipeline:       pipeline.New(blockInterceptor{err: &serverbudget.ExceededError{Spent: money.MustParseDollars("10"), Limit: money.MustParseDollars("5"), Period: "monthly", Subject: "a1"}}),
			expectUpstream: false,
		},
		{
			name:           "upstream_error",
			path:           "/v1/openai/chat/completions",
			body:           []byte(`{"model":"gpt-4o","messages":[]}`),
			expectStatus:   http.StatusBadGateway,
			expectCode:     "gateway.upstream_error",
			pipeline:       pipeline.New(),
			expectUpstream: true,
			upstreamStatus: http.StatusInternalServerError,
			upstreamBody:   []byte(`{"error":"boom"}`),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			app := *baseApp
			g := New(&app, nil, tt.pipeline, NewRegistry(), true, nil)

			var upstreamHits int32
			if tt.expectUpstream {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(&upstreamHits, 1)
					if tt.expectPath != "" && r.URL.Path != tt.expectPath {
						t.Fatalf("got upstream path %q want %q", r.URL.Path, tt.expectPath)
					}
					switch tt.name {
					case "raw_openai", "framework_claude_code", "upstream_error":
						if got := r.Header.Get("Authorization"); got != "Bearer real-key" {
							t.Fatalf("got authorization %q", got)
						}
					case "raw_anthropic":
						if got := r.Header.Get("X-API-Key"); got != "real-key" {
							t.Fatalf("got x-api-key %q", got)
						}
					case "raw_google":
						if got := r.URL.Query().Get("key"); got != "real-key" {
							t.Fatalf("got key query %q", got)
						}
					}
					w.WriteHeader(tt.upstreamStatus)
					_, _ = w.Write(tt.upstreamBody)
				}))
				defer upstream.Close()
				g.transport.client.Transport = rewriteTransport(t, upstream.URL)
			} else {
				g.transport.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
					t.Fatalf("upstream should not be called")
					return nil, nil
				})
			}

			mux := http.NewServeMux()
			g.RegisterRoutes(mux)

			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer real-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Fatalf("got status %d want %d body=%s", w.Code, tt.expectStatus, w.Body.String())
			}
			if tt.expectCode != "" {
				var envelope contract.ErrorEnvelope
				if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
					t.Fatalf("decode error envelope: %v body=%s", err, w.Body.String())
				}
				if envelope.Error.Code != tt.expectCode {
					t.Fatalf("got code %q want %q", envelope.Error.Code, tt.expectCode)
				}
			}
			if !tt.expectUpstream && atomic.LoadInt32(&upstreamHits) != 0 {
				t.Fatalf("unexpected upstream hit")
			}
		})
	}
}
