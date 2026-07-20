package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/server/internal/v2/httpapi"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

type fakeAuthenticator struct {
	identity httpapi.Identity
	err      error
	raw      string
}

func (a *fakeAuthenticator) AuthenticateRaw(_ context.Context, raw string) (httpapi.Identity, error) {
	a.raw = raw
	return a.identity, a.err
}

type fakeProviderStore struct {
	grant       provider.Grant
	beginErr    error
	startErr    error
	renewErr    error
	completeErr error
	begin       []provider.BeginRequest
	starts      []provider.AttemptRequest
	renews      []provider.Grant
	completes   []provider.CompleteRequest
}

func (s *fakeProviderStore) Begin(_ context.Context, req provider.BeginRequest) (provider.Grant, error) {
	s.begin = append(s.begin, req)
	return s.grant, s.beginErr
}
func (s *fakeProviderStore) StartAttempt(_ context.Context, req provider.AttemptRequest) error {
	s.starts = append(s.starts, req)
	return s.startErr
}
func (s *fakeProviderStore) RenewLease(_ context.Context, grant provider.Grant) error {
	s.renews = append(s.renews, grant)
	return s.renewErr
}
func (s *fakeProviderStore) Complete(_ context.Context, req provider.CompleteRequest) error {
	s.completes = append(s.completes, req)
	return s.completeErr
}

func gatewayIdentity() httpapi.Identity {
	return httpapi.Identity{
		AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/runtime",
		Operations: []corev2.Operation{corev2.OperationInvoke}, AllowedAgentIDs: []corev2.Ref{"agt_1"},
		CanAssertScope: true,
	}
}

func scopedRequest(t *testing.T, path, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer kv2_prefix.secret")
	req.Header.Set(HeaderTenant, "clinic/42")
	req.Header.Set(HeaderBillTo, "clinic/42")
	req.Header.Set(HeaderActor, "user/7")
	req.Header.Set(HeaderSession, "run/99")
	req.Header.Set(HeaderFeature, "assistant")
	req.Header.Set(HeaderInvocationKey, "run/99:llm/1")
	return req
}

func TestGatewayJSONInvocationIsScopedSanitizedAndSettled(t *testing.T) {
	var providerRequest *http.Request
	var providerBody map[string]any
	var providerBodyBytes int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerRequest = r.Clone(r.Context())
		raw, _ := io.ReadAll(r.Body)
		providerBodyBytes = len(raw)
		if err := json.Unmarshal(raw, &providerBody); err != nil {
			t.Errorf("provider body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "provider-request-1")
		w.Header().Set("Set-Cookie", "provider-secret=bad")
		_, _ = io.WriteString(w, `{"model":"gpt-safe","usage":{"prompt_tokens":12,"completion_tokens":3}}`)
	}))
	defer upstream.Close()

	auth := &fakeAuthenticator{identity: gatewayIdentity()}
	store := &fakeProviderStore{grant: provider.Grant{
		InvocationID: "ivk_1", AttemptNo: 1, AccountID: "account/acme", NamespaceID: "namespace/prod",
		ServiceKeyID: "key/runtime", AgentID: "agt_1", RouteID: "rte_1",
		Provider: "openai", BaseURL: upstream.URL + "/v1", Model: "gpt-safe",
		Credential: []byte("provider-key"),
		Price:      &provider.Price{Revision: 3, InputNanosPerMillionTokens: 2_000_000, OutputNanosPerMillionTokens: 4_000_000},
	}}
	handler, err := newWithTransport(auth, store, http.DefaultTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	if err := Register(mux, handler); err != nil {
		t.Fatal(err)
	}

	req := scopedRequest(t, "/v2/agents/clinic-assistant/openai/chat/completions", `{"model":"gpt-safe","stream":true,"n":3,"max_completion_tokens":20,"messages":[{"role":"user","content":"<&>"}]}`)
	req.Header.Set("Cookie", "session=secret")
	req.Header.Set("Api-Key", "caller-provider-key")
	req.Header.Set("Traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if auth.raw != "kv2_prefix.secret" {
		t.Fatalf("authenticated raw key = %q", auth.raw)
	}
	if providerRequest == nil {
		t.Fatal("provider request was not made")
	}
	if providerRequest.URL.Path != "/v1/chat/completions" || providerRequest.URL.RawQuery != "" {
		t.Fatalf("provider URL = %s", providerRequest.URL)
	}
	if got := providerRequest.Header.Get("Authorization"); got != "Bearer provider-key" {
		t.Fatalf("provider auth = %q", got)
	}
	if providerRequest.Header.Get("Cookie") != "" || providerRequest.Header.Get("Api-Key") != "" || providerRequest.Header.Get(HeaderTenant) != "" {
		t.Fatalf("caller credentials/scope leaked upstream: %#v", providerRequest.Header)
	}
	if providerRequest.Header.Get("Idempotency-Key") == "" {
		t.Fatal("stable provider idempotency key missing")
	}
	if providerRequest.Header.Get("Traceparent") == "" {
		t.Fatal("traceparent not preserved")
	}
	if providerBody["model"] != "gpt-safe" {
		t.Fatalf("provider model = %#v", providerBody["model"])
	}
	streamOptions, _ := providerBody["stream_options"].(map[string]any)
	if streamOptions["include_usage"] != true {
		t.Fatalf("stream_options = %#v", streamOptions)
	}
	if recorder.Header().Get("Set-Cookie") != "" {
		t.Fatal("provider cookie leaked to caller")
	}
	if recorder.Header().Get("X-Kave-Invocation-ID") != "ivk_1" {
		t.Fatalf("invocation header = %q", recorder.Header().Get("X-Kave-Invocation-ID"))
	}

	if len(store.begin) != 1 || len(store.starts) != 1 || len(store.completes) != 1 {
		t.Fatalf("store calls: begin=%d start=%d complete=%d", len(store.begin), len(store.starts), len(store.completes))
	}
	begin := store.begin[0]
	if begin.Scope.Tenant != "clinic/42" || begin.Scope.BillTo != "clinic/42" || begin.InvocationKey != "run/99:llm/1" {
		t.Fatalf("begin scope = %#v", begin)
	}
	if begin.InputUpperBound < int64(providerBodyBytes) || !begin.InputBounded || begin.OutputUpperBound != 60 || !begin.OutputBounded {
		t.Fatalf("bounds = %#v", begin)
	}
	complete := store.completes[0]
	if !complete.Usage.Reported || complete.Usage.InputTokens != 12 || complete.Usage.OutputTokens != 3 || complete.Usage.CostNanos != 36 || complete.Uncertain {
		t.Fatalf("complete = %#v", complete)
	}
	if complete.ProviderRequestID != "provider-request-1" {
		t.Fatalf("provider request id = %q", complete.ProviderRequestID)
	}
}

func TestOutputUpperBoundRejectsInvalidOrOverflowingChoiceCounts(t *testing.T) {
	t.Parallel()
	for name, document := range map[string]map[string]any{
		"fractional": {"max_completion_tokens": json.Number("10"), "n": json.Number("1.5")},
		"zero":       {"max_completion_tokens": json.Number("10"), "n": json.Number("0")},
		"overflow":   {"max_completion_tokens": json.Number("9223372036854775807"), "n": json.Number("2")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := outputUpperBound(provider.EndpointChatCompletions, document); err == nil {
				t.Fatal("outputUpperBound succeeded")
			}
		})
	}
}

func TestGatewayStreamsSSEAndCapturesTerminalUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"model\":\"gpt-safe\",\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	store := &fakeProviderStore{grant: provider.Grant{
		InvocationID: "ivk_sse", AttemptNo: 1, AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/runtime",
		AgentID: "agt_1", RouteID: "rte_1", Provider: "openai", BaseURL: upstream.URL, Model: "gpt-safe", Credential: []byte("key"),
	}}
	handler, _ := newWithTransport(&fakeAuthenticator{identity: gatewayIdentity()}, store, http.DefaultTransport, nil)
	mux := http.NewServeMux()
	_ = Register(mux, handler)
	req := scopedRequest(t, "/v2/agents/clinic-assistant/openai/responses", `{"model":"gpt-safe","stream":true,"max_output_tokens":8,"input":"hello"}`)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "[DONE]") {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
	if len(store.completes) != 1 || store.completes[0].Usage.InputTokens != 7 || store.completes[0].Usage.OutputTokens != 2 || store.completes[0].Uncertain {
		t.Fatalf("settlement = %#v", store.completes)
	}
}

func TestGatewayChargesConservativelyWhenProviderReportsAnotherModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"unvalidated-model","usage":{"prompt_tokens":7,"completion_tokens":2}}`)
	}))
	defer upstream.Close()
	store := &fakeProviderStore{grant: provider.Grant{
		InvocationID: "ivk_model", AttemptNo: 1, AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/runtime",
		AgentID: "agt_1", RouteID: "rte_1", Provider: "openai", BaseURL: upstream.URL,
		Model: "gpt-safe", Credential: []byte("key"), Price: &provider.Price{InputNanosPerMillionTokens: 1, OutputNanosPerMillionTokens: 1},
	}}
	handler, _ := newWithTransport(&fakeAuthenticator{identity: gatewayIdentity()}, store, http.DefaultTransport, nil)
	mux := http.NewServeMux()
	_ = Register(mux, handler)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, scopedRequest(t, "/v2/agents/a/openai/responses", `{"model":"gpt-safe","max_output_tokens":8,"input":"hello"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if len(store.completes) != 1 || !store.completes[0].Uncertain || store.completes[0].Usage.Reported || store.completes[0].Usage.Model != "gpt-safe" {
		t.Fatalf("settlement = %#v", store.completes)
	}
}

func TestGatewayRejectsBeforeStoreOnMissingOrDuplicateScope(t *testing.T) {
	for name, mutate := range map[string]func(*http.Request){
		"missing invocation": func(r *http.Request) { r.Header.Del(HeaderInvocationKey) },
		"duplicate tenant":   func(r *http.Request) { r.Header.Add(HeaderTenant, "clinic/other") },
		"query":              func(r *http.Request) { r.URL.RawQuery = "api-version=unsafe" },
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeProviderStore{}
			handler, _ := newWithTransport(&fakeAuthenticator{identity: gatewayIdentity()}, store, http.DefaultTransport, nil)
			mux := http.NewServeMux()
			_ = Register(mux, handler)
			req := scopedRequest(t, "/v2/agents/a/openai/embeddings", `{"model":"embed","input":"hello"}`)
			mutate(req)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", recorder.Code)
			}
			if len(store.begin) != 0 {
				t.Fatal("store was called")
			}
		})
	}
}

func TestGatewayRequiresInvokeAndMapsAdmissionFailures(t *testing.T) {
	identity := gatewayIdentity()
	identity.Operations = []corev2.Operation{corev2.OperationConsume}
	handler, _ := newWithTransport(&fakeAuthenticator{identity: identity}, &fakeProviderStore{}, http.DefaultTransport, nil)
	mux := http.NewServeMux()
	_ = Register(mux, handler)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, scopedRequest(t, "/v2/agents/a/openai/embeddings", `{"input":"hello"}`))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}

	for name, want := range map[string]struct {
		err    error
		status int
	}{
		"limit":       {corev2.ErrLimitExceeded, http.StatusTooManyRequests},
		"reservation": {provider.ErrReservationUnavailable, http.StatusServiceUnavailable},
		"duplicate":   {provider.ErrAlreadyInvoked, http.StatusConflict},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeProviderStore{beginErr: want.err}
			h, _ := newWithTransport(&fakeAuthenticator{identity: gatewayIdentity()}, store, http.DefaultTransport, nil)
			m := http.NewServeMux()
			_ = Register(m, h)
			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, scopedRequest(t, "/v2/agents/a/openai/embeddings", `{"model":"embed","input":"hello"}`))
			if rr.Code != want.status {
				t.Fatalf("status = %d", rr.Code)
			}
		})
	}
}

func TestGatewayFailsClosedWhenAttemptLedgerUnavailable(t *testing.T) {
	store := &fakeProviderStore{
		grant:    provider.Grant{InvocationID: "ivk", AttemptNo: 1, AccountID: "a", NamespaceID: "n", ServiceKeyID: "k", Credential: []byte("secret"), BaseURL: "https://example.com", Model: "m"},
		startErr: errors.New("database unavailable"),
	}
	handler, _ := newWithTransport(&fakeAuthenticator{identity: gatewayIdentity()}, store, http.DefaultTransport, nil)
	mux := http.NewServeMux()
	_ = Register(mux, handler)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, scopedRequest(t, "/v2/agents/a/openai/embeddings", `{"model":"m","input":"hello"}`))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", recorder.Code)
	}
	if len(store.completes) != 1 || store.completes[0].Uncertain {
		t.Fatalf("abort settlement = %#v", store.completes)
	}
}

func TestGatewayCancelsProviderWhenLeaseCannotRenew(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer upstream.Close()
	store := &fakeProviderStore{
		grant: provider.Grant{
			InvocationID: "ivk_lease", AttemptNo: 1, AccountID: "account/acme", NamespaceID: "namespace/prod",
			ServiceKeyID: "key/runtime", AgentID: "agt_1", RouteID: "rte_1", Provider: "openai",
			BaseURL: upstream.URL, Model: "embed", Credential: []byte("provider-secret"),
		},
		renewErr: errors.New("accounting unavailable"),
	}
	handler, _ := newWithTransport(&fakeAuthenticator{identity: gatewayIdentity()}, store, http.DefaultTransport, nil)
	handler.renewEvery = time.Millisecond
	handler.renewTimeout = time.Second
	mux := http.NewServeMux()
	_ = Register(mux, handler)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, scopedRequest(t, "/v2/agents/a/openai/embeddings", `{"model":"embed","input":"hello"}`))
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", recorder.Code)
	}
	if len(store.renews) == 0 || len(store.completes) != 1 || !store.completes[0].Uncertain || !store.completes[0].DeliveryStarted {
		t.Fatalf("renewals=%d completions=%#v", len(store.renews), store.completes)
	}
}

func TestProviderURLSecurity(t *testing.T) {
	for _, base := range []string{"http://example.com/v1", "https://user:pass@example.com/v1", "https://example.com/v1?key=secret", "file:///tmp"} {
		if _, err := providerURL(base, provider.EndpointResponses); err == nil {
			t.Fatalf("providerURL(%q) succeeded", base)
		}
	}
	if got, err := providerURL("http://127.0.0.1:8080/v1/", provider.EndpointEmbeddings); err != nil || got != "http://127.0.0.1:8080/v1/embeddings" {
		t.Fatalf("loopback URL = %q, %v", got, err)
	}
}

func TestTailCaptureRetainsEnd(t *testing.T) {
	capture := newTailCapture(5)
	_, _ = capture.Write([]byte("1234"))
	_, _ = capture.Write([]byte("567"))
	if got := string(capture.Bytes()); got != "34567" {
		t.Fatalf("tail = %q", got)
	}
}

func TestCalculateCostRounding(t *testing.T) {
	cost, ok := provider.CalculateUsageCost(provider.Price{InputNanosPerMillionTokens: 1, OutputNanosPerMillionTokens: 1}, provider.Usage{InputTokens: 1})
	if !ok || cost != 1 {
		t.Fatalf("cost = %d, %v", cost, ok)
	}
	if _, ok := provider.CalculateUsageCost(provider.Price{InputNanosPerMillionTokens: math.MaxInt64}, provider.Usage{InputTokens: math.MaxInt64}); ok {
		t.Fatal("overflowing cost was accepted")
	}
	if _, ok := provider.CalculateUsageCost(provider.Price{InputNanosPerMillionTokens: 1}, provider.Usage{InputTokens: -1}); ok {
		t.Fatal("negative token usage was accepted")
	}
}
