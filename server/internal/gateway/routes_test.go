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

type captureInterceptor struct {
	action *runtime.Action
}

func (c *captureInterceptor) Before(_ context.Context, action *runtime.Action) (*runtime.Action, error) {
	return action, nil
}

func (c *captureInterceptor) After(_ context.Context, action *runtime.Action, _ *pipeline.Result) error {
	c.action = action
	return nil
}

func (c *captureInterceptor) Name() string { return "capture" }

func TestGatewayRoutes(t *testing.T) {
	t.Setenv("KAVE_TEST_PROVIDER_KEY", "real-key")

	baseApp := &mockAppStore{
		agent: &controlmodel.Agent{ID: "a1", ProjectID: "proj1", EnvID: "default", Status: controlmodel.AgentStatusActive},
		env:   &controlmodel.Environment{ID: "default", ProjectID: "proj1", Name: "default", Slug: "default", Type: "dev", TrustMode: controlmodel.TrustPermissive},
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
			name:           "raw_openai_responses",
			path:           "/v1/openai/responses",
			body:           []byte(`{"model":"gpt-4o","input":"ping"}`),
			expectPath:     "/v1/responses",
			expectStatus:   http.StatusOK,
			pipeline:       pipeline.New(),
			expectUpstream: true,
			upstreamStatus: http.StatusOK,
			upstreamBody:   []byte(`{"model":"gpt-4o","usage":{"input_tokens":10,"output_tokens":20}}`),
		},
		{
			name:           "github_tool_read",
			path:           "/v1/tools/github/repos/kave-io/kave",
			expectPath:     "/repos/kave-io/kave",
			expectStatus:   http.StatusOK,
			pipeline:       pipeline.New(),
			expectUpstream: true,
			upstreamStatus: http.StatusOK,
			upstreamBody:   []byte(`{"full_name":"kave-io/kave"}`),
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
			path:           "/v1/openai/chat/completions",
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
					case "raw_openai", "raw_openai_responses", "upstream_error", "github_tool_read":
						if got := r.Header.Get("Authorization"); got != "Bearer real-key" {
							t.Fatalf("got authorization %q", got)
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
				var envelope ErrorEnvelope
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

func TestGatewayRejectsDirectOpenAIAliases(t *testing.T) {
	g := New(&mockAppStore{}, nil, pipeline.New(), NewRegistry(), true, nil)
	mux := http.NewServeMux()
	g.RegisterRoutes(mux)

	for _, path := range []string{"/v1/responses", "/v1/chat/completions"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s: got status %d want 404", path, w.Code)
		}
	}
}

func TestGatewayRawOpenAIPassthroughAuthorizationWithoutKaveToken(t *testing.T) {
	app := &mockAppStore{
		agent: &controlmodel.Agent{ID: "default", ProjectID: "proj1", EnvID: "default", Name: "default", Status: controlmodel.AgentStatusActive},
		env:   &controlmodel.Environment{ID: "default", ProjectID: "proj1", Name: "default", Slug: "default", Type: "dev", TrustMode: controlmodel.TrustPermissive},
	}
	g := New(app, nil, pipeline.New(), NewRegistry(), true, nil)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-codex-owned" {
			t.Fatalf("got authorization %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"gpt-4o","usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer upstream.Close()
	g.transport.client.Transport = rewriteTransport(t, upstream.URL)

	mux := http.NewServeMux()
	g.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/v1/openai/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4o","messages":[]}`)))
	req.Header.Set("Authorization", "Bearer sk-codex-owned")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d want 200 body=%s", w.Code, w.Body.String())
	}
}

func TestGatewayCodexChatGPTIngressUsesOpenAIConnectorPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("got upstream path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer chatgpt-token" {
			t.Fatalf("got authorization %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"model\":\"gpt-5\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}\n\n"))
	}))
	defer upstream.Close()
	t.Setenv("KAVE_CODEX_CHATGPT_UPSTREAM", upstream.URL)

	capture := &captureInterceptor{}
	app := &mockAppStore{
		agent: &controlmodel.Agent{ID: "default", ProjectID: "proj1", EnvID: "default", Name: "default", Status: controlmodel.AgentStatusActive},
		env:   &controlmodel.Environment{ID: "default", ProjectID: "proj1", Name: "default", Slug: "default", Type: "dev", TrustMode: controlmodel.TrustPermissive},
	}
	g := New(app, nil, pipeline.New(capture), NewRegistry(), true, nil)

	mux := http.NewServeMux()
	g.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/v1/openai/backend-api/codex/responses", bytes.NewReader([]byte(`{"model":"gpt-5","input":"ping","stream":true}`)))
	req.Header.Set("Authorization", "Bearer chatgpt-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d want 200 body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Kave-Ingress-Route"); got != "openai.codex.chatgpt" {
		t.Fatalf("got route header %q", got)
	}
	if got := w.Header().Get("X-Kave-Transport"); got != "sse" {
		t.Fatalf("got transport header %q", got)
	}
	if w.Header().Get("X-Kave-Run-ID") == "" || w.Header().Get("X-Kave-Trace-ID") == "" {
		t.Fatalf("missing kave trace headers: %v", w.Header())
	}
	if capture.action == nil {
		t.Fatal("capture action is nil")
	}
	if got := capture.action.Attrs["ingress.inbound_path"]; got != "/v1/openai/backend-api/codex/responses" {
		t.Fatalf("got inbound path attr %v", got)
	}
	if got := capture.action.Attrs["ingress.upstream_path"]; got != "/backend-api/codex/responses" {
		t.Fatalf("got upstream path attr %v", got)
	}
	if got := capture.action.Attrs["ingress.auth_mode"]; got != "chatgpt_bearer" {
		t.Fatalf("got auth mode attr %v", got)
	}
	if got := capture.action.Attrs["ingress.credential_mode"]; got != "passthrough" {
		t.Fatalf("got credential mode attr %v", got)
	}
}

func TestGatewayCodexChatGPTBaseCompatibilityUsesOpenAIPipeline(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("got upstream path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer chatgpt-token" {
			t.Fatalf("got authorization %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"model\":\"gpt-5\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}\n\n"))
	}))
	defer upstream.Close()
	t.Setenv("KAVE_CODEX_CHATGPT_UPSTREAM", upstream.URL)

	capture := &captureInterceptor{}
	app := &mockAppStore{
		agent: &controlmodel.Agent{ID: "default", ProjectID: "proj1", EnvID: "default", Name: "default", Status: controlmodel.AgentStatusActive},
		env:   &controlmodel.Environment{ID: "default", ProjectID: "proj1", Name: "default", Slug: "default", Type: "dev", TrustMode: controlmodel.TrustPermissive},
	}
	g := New(app, nil, pipeline.New(capture), NewRegistry(), true, nil)

	mux := http.NewServeMux()
	g.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/backend-api/codex/responses", bytes.NewReader([]byte(`{"model":"gpt-5","input":"ping","stream":true}`)))
	req.Header.Set("Authorization", "Bearer chatgpt-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got status %d want 200 body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Kave-Ingress-Route"); got != "openai.codex.chatgpt" {
		t.Fatalf("got route header %q", got)
	}
	if capture.action == nil {
		t.Fatal("capture action is nil")
	}
	if got := capture.action.Attrs["ingress.inbound_path"]; got != "/backend-api/codex/responses" {
		t.Fatalf("got inbound path attr %v", got)
	}
	if got := capture.action.Attrs["connector.path"]; got != "/v1/openai" {
		t.Fatalf("got connector path attr %v", got)
	}
}

func TestGatewayChatGPTAppsMCPPassthroughIsExactAndOpaque(t *testing.T) {
	tests := []struct {
		inboundPath  string
		upstreamPath string
	}{
		{inboundPath: "/backend-api/wham/apps", upstreamPath: "/backend-api/wham/apps"},
		{inboundPath: "/backend-api/codex/wham/apps", upstreamPath: "/backend-api/wham/apps"},
		{inboundPath: "/connectors/directory/list", upstreamPath: "/connectors/directory/list"},
		{inboundPath: "/connectors/directory/list_workspace", upstreamPath: "/connectors/directory/list_workspace"},
	}

	for _, tt := range tests {
		t.Run(tt.inboundPath, func(t *testing.T) {
			var upstreamHits int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&upstreamHits, 1)
				if r.URL.Path != tt.upstreamPath {
					t.Fatalf("got upstream path %q", r.URL.Path)
				}
				if r.URL.RawQuery != "session=1" {
					t.Fatalf("got upstream query %q", r.URL.RawQuery)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer chatgpt-token" {
					t.Fatalf("got authorization %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
			}))
			defer upstream.Close()
			t.Setenv("KAVE_CODEX_CHATGPT_UPSTREAM", upstream.URL)

			g := New(&mockAppStore{}, nil, pipeline.New(blockInterceptor{err: &serverpolicy.BlockedError{Reason: "should not run"}}), NewRegistry(), true, nil)
			mux := http.NewServeMux()
			g.RegisterRoutes(mux)

			req := httptest.NewRequest(http.MethodPost, tt.inboundPath+"?session=1", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)))
			req.Header.Set("Authorization", "Bearer chatgpt-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("got status %d want 200 body=%s", w.Code, w.Body.String())
			}
			if got := w.Body.String(); got != `{"jsonrpc":"2.0","id":1,"result":{}}` {
				t.Fatalf("got body %q", got)
			}
			if w.Header().Get("X-Kave-Run-ID") != "" {
				t.Fatalf("apps passthrough should not create ingress trace headers: %v", w.Header())
			}
			if atomic.LoadInt32(&upstreamHits) != 1 {
				t.Fatalf("got upstream hits %d want 1", upstreamHits)
			}

			req = httptest.NewRequest(http.MethodPost, tt.inboundPath+"/extra", nil)
			w = httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusNotFound {
				t.Fatalf("got status %d want 404", w.Code)
			}
		})
	}
}
