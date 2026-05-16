package cert

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kave-io/kave/core/connectors"
	connruntime "github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/runtime"
)

// LLMSpec drives RunLLM. Connector is the unit under test; Descriptor exposes
// its declared Capabilities (typically the same value as Connector when the
// connector implements connectors.Descriptor). Cases is the fixture set used
// by the request/auth/parse/streaming checks. GoldenDir, if non-empty, turns
// on per-case golden contract assertions for PreparedRequest + parsed Result.
type LLMSpec struct {
	Name       string
	Connector  connruntime.LLMConnector
	Descriptor connectors.Descriptor
	Cases      []LLMCase
	GoldenDir  string
}

// LLMCase is a single request/response fixture exercising one connector path.
type LLMCase struct {
	Name string

	Call       *connruntime.LLMCall
	Credential string

	// Request expectations.
	ExpectMethod       string   // optional; defaults to Call.Method
	ExpectURLContains  string   // substring the prepared URL must contain
	ExpectAuthHeader   string   // optional; defaults to "Bearer "+Credential when Credential != ""
	ExpectAuthMissing  bool     // when true, assert no Authorization header is set
	ExpectStripHeaders []string // headers that must not appear on the prepared request
	ExpectRequireCreds bool     // when true, expect ErrCredentialRequired with Credential == ""

	// Response.
	ResponseBody []byte
	ResponseType string // "application/json", "text/event-stream", ...
	Streaming    bool   // SSE aggregation across multiple chunks

	// Parse expectations.
	ExpectModel       string
	ExpectInputTok    int
	ExpectOutputTok   int
	ExpectCacheRead   int
	ExpectReasoning   int
	ExpectToolCalls   []ToolCallExpect
	ExpectNoToolCalls bool
}

// ToolCallExpect describes a single tool call the parser must surface as an
// ObservedSpan. ArgsContain is matched as a substring of the span Input bytes.
type ToolCallExpect struct {
	Name        string
	ArgsContain string
}

// RunLLM runs the connector certification suite against an LLM connector.
// Each check is a sub-test so failures pinpoint the gap.
func RunLLM(t *testing.T, spec LLMSpec) {
	t.Helper()
	if spec.Connector == nil {
		t.Fatal("cert: Spec.Connector is nil")
	}

	t.Run("01_capability_shape", func(t *testing.T) { checkLLMCapabilityShape(t, spec) })
	t.Run("02_upstream_path_shape", func(t *testing.T) { checkLLMUpstreamPathShape(t, spec) })
	t.Run("03_request_preparation", func(t *testing.T) { checkLLMRequestPreparation(t, spec) })
	t.Run("04_auth_injection", func(t *testing.T) { checkLLMAuthInjection(t, spec) })
	t.Run("05_auth_stripping", func(t *testing.T) { checkLLMAuthStripping(t, spec) })
	t.Run("06_usage_parsing", func(t *testing.T) { checkLLMUsageParsing(t, spec) })
	t.Run("07_streaming_aggregation", func(t *testing.T) { checkLLMStreaming(t, spec) })
	t.Run("08_tool_call_extraction", func(t *testing.T) { checkLLMToolCalls(t, spec) })
	t.Run("09_policy_contract", func(t *testing.T) { checkLLMPolicyContract(t, spec) })
	t.Run("10_budget_contract", func(t *testing.T) { checkLLMBudgetContract(t, spec) })
	t.Run("11_error_tolerance", func(t *testing.T) { checkLLMErrorTolerance(t, spec) })

	if spec.GoldenDir != "" {
		t.Run("12_golden_contract", func(t *testing.T) { checkLLMGolden(t, spec) })
	}
}

// ── individual checks ────────────────────────────────────────────────────────

func checkLLMCapabilityShape(t *testing.T, spec LLMSpec) {
	t.Helper()
	if spec.Descriptor == nil {
		t.Skip("no Descriptor supplied; skipping capability shape")
	}
	caps := spec.Descriptor.Capabilities()
	if caps.Kind != connectors.KindLLM {
		t.Errorf("capability Kind = %q, want %q", caps.Kind, connectors.KindLLM)
	}
	if len(caps.SupportedMethods) == 0 {
		t.Error("capability SupportedMethods is empty — connector cannot advertise its surface")
	}
	if len(caps.SupportedRoutes) == 0 {
		t.Error("capability SupportedRoutes is empty — gateway has nothing to mount")
	}
	if caps.APIVersion == "" {
		t.Error("capability APIVersion is empty — required for upstream version pinning")
	}
	if caps.RequiresAuth != spec.Connector.RequiresAuth() {
		t.Errorf("capability RequiresAuth=%v, connector.RequiresAuth()=%v — must agree",
			caps.RequiresAuth, spec.Connector.RequiresAuth())
	}
}

// checkLLMUpstreamPathShape asserts every Case.Call.UpstreamPath is a real
// post-strip upstream path the connector can forward verbatim — not an inbound
// Kave gateway path. The gateway is responsible for translating inbound paths
// (matching Capabilities.SupportedRoutes) into upstream paths before invoking
// the connector; cert tests start from a parsed LLMCall.
func checkLLMUpstreamPathShape(t *testing.T, spec LLMSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		if c.Call == nil {
			continue
		}
		p := c.Call.UpstreamPath
		if p == "" {
			t.Errorf("case %q: UpstreamPath empty", c.Name)
			continue
		}
		if !strings.HasPrefix(p, "/") {
			t.Errorf("case %q: UpstreamPath %q must start with /", c.Name, p)
		}
		// Catch the most common mistake: fixtures that still carry the inbound
		// Kave gateway prefix instead of the real upstream path.
		if strings.HasPrefix(p, "/v1/openai/") || strings.HasPrefix(p, "/v1/anthropic/") ||
			strings.HasPrefix(p, "/v1/gemini/") || strings.HasPrefix(p, "/v1/ollama/") ||
			strings.HasPrefix(p, "/v1/tools/") {
			t.Errorf("case %q: UpstreamPath %q looks like an inbound Kave route — fixtures should use the post-strip upstream path the gateway forwards",
				c.Name, p)
		}
	}
}

func checkLLMRequestPreparation(t *testing.T, spec LLMSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call == nil {
				t.Skip("no Call fixture")
			}
			prep, err := spec.Connector.PrepareRequest(c.Call, c.Credential)
			if c.ExpectRequireCreds && c.Credential == "" {
				if !errors.Is(err, connruntime.ErrCredentialRequired) {
					t.Errorf("expected ErrCredentialRequired, got err=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("PrepareRequest: %v", err)
			}
			if prep == nil {
				t.Fatal("PrepareRequest returned nil prep without error")
			}
			wantMethod := c.ExpectMethod
			if wantMethod == "" {
				wantMethod = c.Call.Method
			}
			if wantMethod != "" && prep.Method != wantMethod {
				t.Errorf("prep.Method=%q want %q", prep.Method, wantMethod)
			}
			if c.ExpectURLContains != "" && !strings.Contains(prep.URL, c.ExpectURLContains) {
				t.Errorf("prep.URL=%q does not contain %q", prep.URL, c.ExpectURLContains)
			}
			if !bytesEqual(prep.Body, c.Call.Body) {
				t.Errorf("prep.Body mutated by PrepareRequest (len %d → %d)", len(c.Call.Body), len(prep.Body))
			}
		})
	}
}

func checkLLMAuthInjection(t *testing.T, spec LLMSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call == nil || (c.Credential == "" && !c.ExpectAuthMissing) {
				t.Skip("no credential to inject")
			}
			prep, err := spec.Connector.PrepareRequest(c.Call, c.Credential)
			if err != nil || prep == nil {
				t.Skipf("prep failed (covered by request_preparation): %v", err)
			}
			got := prep.Header.Get("Authorization")
			switch {
			case c.ExpectAuthMissing:
				if got != "" {
					t.Errorf("Authorization=%q, expected absent", got)
				}
			case c.ExpectAuthHeader != "":
				if got != c.ExpectAuthHeader {
					t.Errorf("Authorization=%q want %q", got, c.ExpectAuthHeader)
				}
			default:
				want := "Bearer " + strings.TrimPrefix(c.Credential, "Bearer ")
				if got != want {
					t.Errorf("Authorization=%q want %q", got, want)
				}
			}
		})
	}
}

func checkLLMAuthStripping(t *testing.T, spec LLMSpec) {
	t.Helper()
	// hop-by-hop headers (RFC 7230 §6.1) must never reach upstream.
	hop := []string{"Connection", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding"}
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call == nil {
				t.Skip("no Call fixture")
			}
			// Plant inbound noise to confirm the connector scrubs it.
			call := *c.Call
			call.Header = connruntime.CloneHeader(c.Call.Header)
			call.Header.Set("Authorization", "Bearer inbound-leak-token")
			call.Header.Set("Connection", "keep-alive")
			call.Header.Set("Proxy-Authorization", "Basic leak")
			prep, err := spec.Connector.PrepareRequest(&call, c.Credential)
			if err != nil || prep == nil {
				t.Skipf("prep failed: %v", err)
			}
			if got := prep.Header.Get("Authorization"); strings.Contains(got, "inbound-leak-token") {
				t.Errorf("inbound Authorization leaked to upstream: %q", got)
			}
			for _, h := range hop {
				if v := prep.Header.Get(h); v != "" {
					t.Errorf("hop-by-hop header %q leaked: %q", h, v)
				}
			}
			for _, h := range c.ExpectStripHeaders {
				if v := prep.Header.Get(h); v != "" {
					t.Errorf("expected header %q to be stripped, got %q", h, v)
				}
			}
		})
	}
}

func checkLLMUsageParsing(t *testing.T, spec LLMSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if len(c.ResponseBody) == 0 {
				t.Skip("no ResponseBody fixture")
			}
			result, err := spec.Connector.ParseResponse(c.ResponseBody, c.ResponseType)
			if err != nil {
				t.Fatalf("ParseResponse: %v", err)
			}
			if c.ExpectInputTok == 0 && c.ExpectOutputTok == 0 && c.ExpectModel == "" {
				return // case isn't asserting usage
			}
			if result.TokenUsage == nil {
				t.Fatal("TokenUsage is nil; expected usage parse to populate it")
			}
			u := result.TokenUsage
			if c.ExpectModel != "" && u.Model != c.ExpectModel {
				t.Errorf("Model=%q want %q", u.Model, c.ExpectModel)
			}
			if c.ExpectInputTok != 0 && u.InputTokens != c.ExpectInputTok {
				t.Errorf("InputTokens=%d want %d", u.InputTokens, c.ExpectInputTok)
			}
			if c.ExpectOutputTok != 0 && u.OutputTokens != c.ExpectOutputTok {
				t.Errorf("OutputTokens=%d want %d", u.OutputTokens, c.ExpectOutputTok)
			}
			if c.ExpectCacheRead != 0 && u.CacheRead != c.ExpectCacheRead {
				t.Errorf("CacheRead=%d want %d", u.CacheRead, c.ExpectCacheRead)
			}
			if c.ExpectReasoning != 0 && u.Reasoning != c.ExpectReasoning {
				t.Errorf("Reasoning=%d want %d", u.Reasoning, c.ExpectReasoning)
			}
		})
	}
}

func checkLLMStreaming(t *testing.T, spec LLMSpec) {
	t.Helper()
	anyCase := false
	for _, c := range spec.Cases {
		if !c.Streaming || len(c.ResponseBody) == 0 {
			continue
		}
		anyCase = true
		c := c
		t.Run(c.Name, func(t *testing.T) {
			result, err := spec.Connector.ParseResponse(c.ResponseBody, c.ResponseType)
			if err != nil {
				t.Fatalf("ParseResponse(stream): %v", err)
			}
			if c.ExpectInputTok != 0 || c.ExpectOutputTok != 0 {
				if result.TokenUsage == nil {
					t.Fatal("streaming usage not aggregated: TokenUsage nil")
				}
				if c.ExpectInputTok != 0 && result.TokenUsage.InputTokens != c.ExpectInputTok {
					t.Errorf("aggregated InputTokens=%d want %d",
						result.TokenUsage.InputTokens, c.ExpectInputTok)
				}
				if c.ExpectOutputTok != 0 && result.TokenUsage.OutputTokens != c.ExpectOutputTok {
					t.Errorf("aggregated OutputTokens=%d want %d",
						result.TokenUsage.OutputTokens, c.ExpectOutputTok)
				}
			}
			for _, want := range c.ExpectToolCalls {
				if !hasToolCall(result.ObservedSpans, want) {
					t.Errorf("streaming did not aggregate tool call %q with args containing %q",
						want.Name, want.ArgsContain)
				}
			}
		})
	}
	if !anyCase {
		t.Skip("no streaming cases in spec")
	}
}

func checkLLMToolCalls(t *testing.T, spec LLMSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if len(c.ResponseBody) == 0 || (len(c.ExpectToolCalls) == 0 && !c.ExpectNoToolCalls) {
				t.Skip("no tool call expectations")
			}
			result, err := spec.Connector.ParseResponse(c.ResponseBody, c.ResponseType)
			if err != nil {
				t.Fatalf("ParseResponse: %v", err)
			}
			if c.ExpectNoToolCalls {
				if len(result.ObservedSpans) != 0 {
					t.Errorf("expected no tool calls, got %d", len(result.ObservedSpans))
				}
				return
			}
			for _, want := range c.ExpectToolCalls {
				if !hasToolCall(result.ObservedSpans, want) {
					t.Errorf("missing tool call: name=%q args~%q (got spans=%v)",
						want.Name, want.ArgsContain, spanSummary(result.ObservedSpans))
				}
			}
		})
	}
}

// checkLLMPolicyContract asserts the PreparedRequest carries every field the
// gateway needs to make a policy decision before egress.
func checkLLMPolicyContract(t *testing.T, spec LLMSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call == nil {
				t.Skip("no Call fixture")
			}
			prep, err := spec.Connector.PrepareRequest(c.Call, c.Credential)
			if err != nil || prep == nil {
				t.Skipf("prep failed: %v", err)
			}
			if prep.Method == "" {
				t.Error("policy contract: prep.Method empty (policy cannot match on method)")
			}
			if prep.URL == "" {
				t.Error("policy contract: prep.URL empty (policy cannot match on destination)")
			}
			if prep.Header == nil {
				t.Error("policy contract: prep.Header nil (policy cannot inspect headers)")
			}
			if !bytesEqual(prep.Body, c.Call.Body) {
				t.Error("policy contract: prep.Body mutated; policy must see the wire bytes")
			}
		})
	}
}

// checkLLMBudgetContract asserts result.TokenUsage and result.Usage.Tokens both
// point at the same numbers — cost engine and budget engine each read one.
func checkLLMBudgetContract(t *testing.T, spec LLMSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if len(c.ResponseBody) == 0 || (c.ExpectInputTok == 0 && c.ExpectOutputTok == 0) {
				t.Skip("no usage-bearing fixture")
			}
			result, err := spec.Connector.ParseResponse(c.ResponseBody, c.ResponseType)
			if err != nil {
				t.Fatalf("ParseResponse: %v", err)
			}
			if result.TokenUsage == nil {
				t.Fatal("budget contract: TokenUsage nil")
			}
			if result.Usage == nil || result.Usage.Tokens == nil {
				t.Fatal("budget contract: Usage.Tokens nil (cost/budget engine cannot meter)")
			}
			if result.Usage.Tokens != result.TokenUsage {
				t.Error("budget contract: Usage.Tokens must alias TokenUsage so both readers see the same numbers")
			}
		})
	}
}

func checkLLMErrorTolerance(t *testing.T, spec LLMSpec) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ParseResponse panicked on garbage body: %v", r)
		}
	}()
	bad := []byte(`{"this is not": "valid usage", "choices": "not-an-array"`)
	if _, err := spec.Connector.ParseResponse(bad, "application/json"); err != nil {
		t.Logf("ParseResponse(garbage) returned error (acceptable): %v", err)
	}
	if _, err := spec.Connector.ParseResponse(nil, "application/json"); err != nil {
		t.Logf("ParseResponse(nil) returned error (acceptable): %v", err)
	}
}

func checkLLMGolden(t *testing.T, spec LLMSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call != nil {
				if prep, err := spec.Connector.PrepareRequest(c.Call, c.Credential); err == nil && prep != nil {
					path := filepath.Join(spec.GoldenDir, c.Name+".request.json")
					assertGoldenJSON(t, path, summarizePrep(prep))
				}
			}
			if len(c.ResponseBody) > 0 {
				if result, err := spec.Connector.ParseResponse(c.ResponseBody, c.ResponseType); err == nil && result != nil {
					path := filepath.Join(spec.GoldenDir, c.Name+".result.json")
					assertGoldenJSON(t, path, summarizeResult(result))
				}
			}
		})
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasToolCall(spans []runtime.ObservedSpan, want ToolCallExpect) bool {
	for _, s := range spans {
		if !matchesToolName(s, want.Name) {
			continue
		}
		if want.ArgsContain == "" {
			return true
		}
		if s.Input != nil && strings.Contains(string(*s.Input), want.ArgsContain) {
			return true
		}
	}
	return false
}

func matchesToolName(s runtime.ObservedSpan, name string) bool {
	if name == "" {
		return true
	}
	if s.Name == name || s.Name == "tool."+name {
		return true
	}
	if tc, ok := s.Attrs["tool_call.name"].(string); ok && tc == name {
		return true
	}
	return false
}

type spanInfo struct {
	Name  string `json:"name"`
	Input string `json:"input,omitempty"`
}

func spanSummary(spans []runtime.ObservedSpan) []spanInfo {
	out := make([]spanInfo, 0, len(spans))
	for _, s := range spans {
		info := spanInfo{Name: s.Name}
		if s.Input != nil {
			info.Input = string(*s.Input)
		}
		out = append(out, info)
	}
	return out
}

type preparedSnapshot struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body,omitempty"`
}

func summarizePrep(p *connruntime.PreparedRequest) preparedSnapshot {
	return preparedSnapshot{
		Method:  p.Method,
		URL:     p.URL,
		Headers: cloneHeaderMap(p.Header),
		Body:    string(p.Body),
	}
}

type resultSnapshot struct {
	BodyLen      int                 `json:"body_len"`
	TokenUsage   *runtime.TokenUsage `json:"token_usage,omitempty"`
	HasUsageLink bool                `json:"has_usage_link"`
	Spans        []spanInfo          `json:"spans,omitempty"`
}

func summarizeResult(r *pipeline.Result) resultSnapshot {
	snap := resultSnapshot{
		BodyLen:    len(r.Body),
		TokenUsage: r.TokenUsage,
		Spans:      spanSummary(r.ObservedSpans),
	}
	if r.Usage != nil && r.Usage.Tokens != nil {
		snap.HasUsageLink = r.Usage.Tokens == r.TokenUsage
	}
	return snap
}

func cloneHeaderMap(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
