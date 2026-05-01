package testutil_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/authhash"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/testutil"
	"github.com/kave-io/kave/server/ops/trace"
)

// staticTransport returns a canned HTTP response to every upstream request.
// Set called to a *bool to record whether the transport was invoked.
type staticTransport struct {
	statusCode  int
	body        []byte
	contentType string
	called      *bool
}

func (s *staticTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	if s.called != nil {
		*s.called = true
	}
	return &http.Response{
		StatusCode: s.statusCode,
		Header:     http.Header{"Content-Type": []string{s.contentType}},
		Body:       io.NopCloser(bytes.NewReader(s.body)),
	}, nil
}

// minimalChatCompletion is a minimal valid OpenAI chat completion response.
const minimalChatCompletion = `{"id":"chatcmpl-test","object":"chat.completion","created":1700000000,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

func chatRequestBody(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var env struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(body, &env)
	return env.Error.Code
}

// TestIntegration_GatewayHappyPath: authenticated POST → upstream mock → 200
// and the trace interceptor records a span in the SpanStore.
func TestIntegration_GatewayHappyPath(t *testing.T) {
	t.Setenv("KAVE_TEST_OPENAI_KEY", "test-key-happy")
	h := testutil.New(t)
	h.Gateway.SetTransport(&staticTransport{
		statusCode:  200,
		body:        []byte(minimalChatCompletion),
		contentType: "application/json",
	})

	req, err := http.NewRequest(http.MethodPost,
		h.Server.URL+"/v1/openai/chat/completions",
		bytes.NewReader(chatRequestBody(t)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+h.RawToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.Client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, respBody)
	}

	// Trace interceptor writes the span synchronously inside pipeline.Execute.
	res, err := h.SpanStore.QuerySpans(context.Background(),
		&runtimemodel.SpanFilter{AgentID: testutil.AgentID},
		store.Page{Limit: 10})
	if err != nil {
		t.Fatalf("QuerySpans: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("no span recorded after successful proxy")
	}
	sp := res.Items[0]
	if sp.AgentID != testutil.AgentID {
		t.Errorf("span.AgentID = %q, want %q", sp.AgentID, testutil.AgentID)
	}
	if sp.Connector != "openai" {
		t.Errorf("span.Connector = %q, want openai", sp.Connector)
	}
	if sp.EndedAt == nil {
		t.Error("span.EndedAt should be set after successful completion")
	}
}

// TestIntegration_GatewayUnauthorized: missing bearer token → 401.
func TestIntegration_GatewayUnauthorized(t *testing.T) {
	h := testutil.New(t)

	req, _ := http.NewRequest(http.MethodPost,
		h.Server.URL+"/v1/openai/chat/completions",
		bytes.NewReader(chatRequestBody(t)))
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header — identity will be anonymous; anon is not allowed
	// with the default env (non-permissive trust mode) so expects 401.

	resp, err := h.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", resp.StatusCode, body)
	}
	// Anonymous requests with no bearer token reach the three-axis anon path.
	// Since there is no permissive "default" env in the harness, the gateway
	// returns env_requires_authentication (still 401).
	if got := errorCode(t, body); got != "gateway.env_requires_authentication" && got != "gateway.unauthorized" {
		t.Errorf("error code = %q, want an auth-required 401 code", got)
	}
}

// TestIntegration_GatewayPolicyBlock: connector absent from AllowedConnectors
// list → 403 gateway.policy_blocked; upstream must not be called.
func TestIntegration_GatewayPolicyBlock(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	// Policy that permits anthropic only — blocks any openai request.
	polID := "pol-restricted-test"
	if err := h.AppStore.CreatePolicy(ctx, &controlmodel.PolicyRecord{
		ID:                polID,
		ProjectID:         testutil.ProjectID,
		EnvID:             testutil.EnvID,
		Name:              "restricted",
		AllowedTypes:      []string{"*"},
		AllowedConnectors: []string{"anthropic"},
		AllowedMethods:    []string{"*"},
		Mode:              "enforce",
		Status:            string(controlmodel.PolicyStatusActive),
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	agentID := "agt-restricted-test"
	if err := h.AppStore.CreateAgent(ctx, &controlmodel.Agent{
		ID:        agentID,
		ProjectID: testutil.ProjectID,
		EnvID:     testutil.EnvID,
		Name:      "restricted-agent",
		PolicyID:  &polID,
		Status:    controlmodel.AgentStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	rawToken, tokenHash, err := authhash.GenerateToken("kv_")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if err := h.AppStore.InsertAgentToken(ctx, &controlmodel.AgentToken{
		ID:        "tok-restricted-test",
		OrgID:     testutil.OrgID,
		AgentID:   agentID,
		ProjectID: testutil.ProjectID,
		Name:      "restricted-token",
		TokenHash: tokenHash,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("InsertAgentToken: %v", err)
	}

	var upstreamCalled bool
	h.Gateway.SetTransport(&staticTransport{
		statusCode:  200,
		body:        []byte(minimalChatCompletion),
		contentType: "application/json",
		called:      &upstreamCalled,
	})

	req, _ := http.NewRequest(http.MethodPost,
		h.Server.URL+"/v1/openai/chat/completions",
		bytes.NewReader(chatRequestBody(t)))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", resp.StatusCode, body)
	}
	if got := errorCode(t, body); got != "gateway.policy_blocked" {
		t.Errorf("error code = %q, want gateway.policy_blocked", got)
	}
	if upstreamCalled {
		t.Error("upstream must not be called when policy blocks the request")
	}
}

// TestIntegration_GatewayBudgetBlock: agent with exhausted monthly budget
// → 402 gateway.budget_exceeded; upstream must not be called.
func TestIntegration_GatewayBudgetBlock(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	cap := money.MustParseAmount("0.01") // $0.01 cap
	polID := "pol-capped-test"
	if err := h.AppStore.CreatePolicy(ctx, &controlmodel.PolicyRecord{
		ID:                polID,
		ProjectID:         testutil.ProjectID,
		EnvID:             testutil.EnvID,
		Name:              "capped",
		AllowedTypes:      []string{"*"},
		AllowedConnectors: []string{"*"},
		AllowedMethods:    []string{"*"},
		BudgetCap:         cap,
		BudgetPeriod:      "monthly",
		Mode:              "enforce",
		Status:            string(controlmodel.PolicyStatusActive),
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	agentID := "agt-capped-test"
	if err := h.AppStore.CreateAgent(ctx, &controlmodel.Agent{
		ID:        agentID,
		ProjectID: testutil.ProjectID,
		EnvID:     testutil.EnvID,
		Name:      "capped-agent",
		PolicyID:  &polID,
		Status:    controlmodel.AgentStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	rawToken, tokenHash, err := authhash.GenerateToken("kv_")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if err := h.AppStore.InsertAgentToken(ctx, &controlmodel.AgentToken{
		ID:        "tok-capped-test",
		OrgID:     testutil.OrgID,
		AgentID:   agentID,
		ProjectID: testutil.ProjectID,
		Name:      "capped-token",
		TokenHash: tokenHash,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("InsertAgentToken: %v", err)
	}

	// budget_ledger.run_id is NOT NULL and FK to runs; create a seed run first.
	seedRunID := "run-capped-seed"
	if err := h.AppStore.CreateRun(ctx, &runtimemodel.RunRecord{
		ID:        seedRunID,
		ProjectID: testutil.ProjectID,
		EnvID:     testutil.EnvID,
		AgentID:   agentID,
		Name:      "seed-run",
		Status:    "completed",
		Metadata:  map[string]any{},
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	// Seed spend above cap: $0.02 > $0.01
	spent := money.MustParseAmount("0.02")
	if err := h.AppStore.InsertBudgetEntry(ctx, &runtimemodel.BudgetEntry{
		ID:        "bge-capped-seed",
		ProjectID: testutil.ProjectID,
		EnvID:     testutil.EnvID,
		AgentID:   agentID,
		RunID:     seedRunID,
		Cost:      spent,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("InsertBudgetEntry: %v", err)
	}

	var upstreamCalled bool
	h.Gateway.SetTransport(&staticTransport{
		statusCode:  200,
		body:        []byte(minimalChatCompletion),
		contentType: "application/json",
		called:      &upstreamCalled,
	})

	req, _ := http.NewRequest(http.MethodPost,
		h.Server.URL+"/v1/openai/chat/completions",
		bytes.NewReader(chatRequestBody(t)))
	req.Header.Set("Authorization", "Bearer "+rawToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d: %s", resp.StatusCode, body)
	}
	if got := errorCode(t, body); got != "gateway.budget_exceeded" {
		t.Errorf("error code = %q, want gateway.budget_exceeded", got)
	}
	if upstreamCalled {
		t.Error("upstream must not be called when budget is exhausted")
	}
}

// minimalSSEStream is a minimal valid OpenAI streaming response.
// Two content chunks plus a usage-bearing chunk, terminated by [DONE].
const minimalSSEStream = "" +
	"data: {\"id\":\"chatcmpl-sse\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n" +
	"data: {\"id\":\"chatcmpl-sse\",\"object\":\"chat.completion.chunk\",\"created\":1700000000,\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"!\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\n" +
	"data: [DONE]\n\n"

func streamRequestBody(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"model":    "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
		"stream":   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestIntegration_GatewaySSE: streaming upstream → chunks forwarded in order →
// span closed with token usage after last chunk.
func TestIntegration_GatewaySSE(t *testing.T) {
	t.Setenv("KAVE_TEST_OPENAI_KEY", "test-key-sse")
	h := testutil.New(t)
	h.Gateway.SetTransport(&staticTransport{
		statusCode:  200,
		body:        []byte(minimalSSEStream),
		contentType: "text/event-stream",
	})

	req, err := http.NewRequest(http.MethodPost,
		h.Server.URL+"/v1/openai/chat/completions",
		bytes.NewReader(streamRequestBody(t)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+h.RawToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.Client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "event-stream") {
		t.Errorf("expected event-stream content-type, got %q", ct)
	}

	// Read all SSE lines from the response body and collect data payloads.
	var dataLines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading SSE body: %v", err)
	}

	if len(dataLines) < 2 {
		t.Fatalf("expected at least 2 SSE data lines, got %d", len(dataLines))
	}
	if dataLines[len(dataLines)-1] != "[DONE]" {
		t.Errorf("last SSE line should be [DONE], got %q", dataLines[len(dataLines)-1])
	}

	// Span is written synchronously inside pipeline.Execute; verify it closed.
	res, err := h.SpanStore.QuerySpans(context.Background(),
		&runtimemodel.SpanFilter{AgentID: testutil.AgentID},
		store.Page{Limit: 10})
	if err != nil {
		t.Fatalf("QuerySpans: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("no span recorded after SSE stream")
	}
	// Find the SSE span (EndedAt set, token usage from streaming usage event).
	var found bool
	for _, sp := range res.Items {
		if sp.EndedAt != nil && sp.Connector == "openai" {
			found = true
			// Verify token usage was parsed from the SSE stream.
			if sp.InputTokens == nil || *sp.InputTokens != 10 {
				var got any = "<nil>"
				if sp.InputTokens != nil {
					got = *sp.InputTokens
				}
				t.Errorf("SSE span InputTokens = %v, want 10", got)
			}
			if sp.OutputTokens == nil || *sp.OutputTokens != 2 {
				var got any = "<nil>"
				if sp.OutputTokens != nil {
					got = *sp.OutputTokens
				}
				t.Errorf("SSE span OutputTokens = %v, want 2", got)
			}
			break
		}
	}
	if !found {
		t.Errorf("no closed openai span found; spans: %d", len(res.Items))
	}
}

// TestIntegration_TraceTree: spans inserted with parent/child relationships
// are queryable and build into a valid tree.
func TestIntegration_TraceTree(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	now := time.Now().UnixMilli()

	rootID := "tree-root-1"
	childID := "tree-child-1"
	grandID := "tree-grand-1"

	if err := h.SpanStore.OpenSpan(ctx, &runtimemodel.SpanRow{
		ID:        rootID,
		RunID:     "tree-run-1",
		AgentID:   "tree-agent",
		ProjectID: testutil.ProjectID,
		EnvID:     testutil.EnvID,
		Name:      "root-span",
		Kind:      "llm",
		Connector: "openai",
		TraceID:   "trace-tree-1",
		StartedAt: now,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("OpenSpan root: %v", err)
	}

	if err := h.SpanStore.OpenSpan(ctx, &runtimemodel.SpanRow{
		ID:        childID,
		RunID:     "tree-run-1",
		AgentID:   "tree-agent",
		ProjectID: testutil.ProjectID,
		EnvID:     testutil.EnvID,
		ParentID:  &rootID,
		Name:      "child-span",
		Kind:      "llm",
		Connector: "openai",
		TraceID:   "trace-tree-1",
		StartedAt: now + 1,
		CreatedAt: now + 1,
	}); err != nil {
		t.Fatalf("OpenSpan child: %v", err)
	}

	if err := h.SpanStore.OpenSpan(ctx, &runtimemodel.SpanRow{
		ID:        grandID,
		RunID:     "tree-run-1",
		AgentID:   "tree-agent",
		ProjectID: testutil.ProjectID,
		EnvID:     testutil.EnvID,
		ParentID:  &childID,
		Name:      "grandchild-span",
		Kind:      "llm",
		Connector: "anthropic",
		TraceID:   "trace-tree-1",
		StartedAt: now + 2,
		CreatedAt: now + 2,
	}); err != nil {
		t.Fatalf("OpenSpan grandchild: %v", err)
	}

	res, err := h.SpanStore.QuerySpans(ctx,
		&runtimemodel.SpanFilter{RunID: "tree-run-1"},
		store.Page{Limit: 50})
	if err != nil {
		t.Fatalf("QuerySpans: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(res.Items))
	}

	tree, err := trace.BuildTree(res.Items)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.Span.ID != rootID {
		t.Errorf("tree root ID = %q, want %q", tree.Span.ID, rootID)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree.Children))
	}
	if tree.Children[0].Span.ID != childID {
		t.Errorf("child ID = %q, want %q", tree.Children[0].Span.ID, childID)
	}
	if len(tree.Children[0].Children) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(tree.Children[0].Children))
	}
	if tree.Children[0].Children[0].Span.ID != grandID {
		t.Errorf("grandchild ID = %q, want %q", tree.Children[0].Children[0].Span.ID, grandID)
	}
}
