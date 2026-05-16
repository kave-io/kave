package cert

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kave-io/kave/core/connectors"
	connruntime "github.com/kave-io/kave/core/connectors/runtime"
	"github.com/kave-io/kave/core/pipeline"
)

// ToolSpec drives RunTool. Connector is the unit under test; Descriptor exposes
// its declared Capabilities. Cases is the fixture set. GoldenDir, if non-empty,
// turns on per-case golden contract assertions.
type ToolSpec struct {
	Name       string
	Connector  connruntime.ToolConnector
	Descriptor connectors.Descriptor
	Cases      []ToolCase
	GoldenDir  string
}

// ToolCase is a single request/response fixture exercising one tool connector path.
type ToolCase struct {
	Name string

	Call       *connruntime.ToolCallRequest
	Credential string

	// Request expectations.
	ExpectMethod       string
	ExpectURLContains  string
	ExpectAuthHeader   string
	ExpectAuthMissing  bool
	ExpectStripHeaders []string
	ExpectRequireCreds bool

	// Response.
	ResponseBody []byte
	ResponseType string

	// Parse expectations.
	ExpectRequestCount int // defaults to 1 when ResponseBody is non-empty
}

// RunTool runs the connector certification suite against a tool connector.
func RunTool(t *testing.T, spec ToolSpec) {
	t.Helper()
	if spec.Connector == nil {
		t.Fatal("cert: Spec.Connector is nil")
	}

	t.Run("01_capability_shape", func(t *testing.T) { checkToolCapabilityShape(t, spec) })
	t.Run("02_upstream_path_shape", func(t *testing.T) { checkToolUpstreamPathShape(t, spec) })
	t.Run("03_request_preparation", func(t *testing.T) { checkToolRequestPreparation(t, spec) })
	t.Run("04_auth_injection", func(t *testing.T) { checkToolAuthInjection(t, spec) })
	t.Run("05_auth_stripping", func(t *testing.T) { checkToolAuthStripping(t, spec) })
	t.Run("06_response_parsing", func(t *testing.T) { checkToolResponseParsing(t, spec) })
	t.Run("07_policy_contract", func(t *testing.T) { checkToolPolicyContract(t, spec) })
	t.Run("08_error_tolerance", func(t *testing.T) { checkToolErrorTolerance(t, spec) })

	if spec.GoldenDir != "" {
		t.Run("09_golden_contract", func(t *testing.T) { checkToolGolden(t, spec) })
	}
}

// ── individual checks ────────────────────────────────────────────────────────

func checkToolCapabilityShape(t *testing.T, spec ToolSpec) {
	t.Helper()
	if spec.Descriptor == nil {
		t.Skip("no Descriptor supplied; skipping capability shape")
	}
	caps := spec.Descriptor.Capabilities()
	if caps.Kind != connectors.KindTool {
		t.Errorf("capability Kind = %q, want %q", caps.Kind, connectors.KindTool)
	}
	if len(caps.SupportedMethods) == 0 {
		t.Error("capability SupportedMethods is empty")
	}
	if len(caps.SupportedRoutes) == 0 {
		t.Error("capability SupportedRoutes is empty")
	}
	if caps.APIVersion == "" {
		t.Error("capability APIVersion is empty")
	}
	if caps.RequiresAuth != spec.Connector.RequiresAuth() {
		t.Errorf("capability RequiresAuth=%v, connector.RequiresAuth()=%v — must agree",
			caps.RequiresAuth, spec.Connector.RequiresAuth())
	}
}

func checkToolUpstreamPathShape(t *testing.T, spec ToolSpec) {
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
		if strings.HasPrefix(p, "/v1/tools/") || strings.HasPrefix(p, "/v1/openai/") ||
			strings.HasPrefix(p, "/v1/anthropic/") {
			t.Errorf("case %q: UpstreamPath %q looks like an inbound Kave route — fixtures should use the post-strip upstream path",
				c.Name, p)
		}
	}
}

func checkToolRequestPreparation(t *testing.T, spec ToolSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call == nil {
				t.Skip("no Call fixture")
			}
			prep, err := spec.Connector.PrepareToolRequest(c.Call, c.Credential)
			if c.ExpectRequireCreds && c.Credential == "" {
				if err == nil {
					t.Error("expected error for missing credential, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("PrepareToolRequest: %v", err)
			}
			if prep == nil {
				t.Fatal("PrepareToolRequest returned nil without error")
			}
			wantMethod := c.ExpectMethod
			if wantMethod == "" {
				wantMethod = c.Call.HTTPMethod
			}
			if wantMethod != "" && prep.Method != wantMethod {
				t.Errorf("prep.Method=%q want %q", prep.Method, wantMethod)
			}
			if c.ExpectURLContains != "" && !strings.Contains(prep.URL, c.ExpectURLContains) {
				t.Errorf("prep.URL=%q does not contain %q", prep.URL, c.ExpectURLContains)
			}
			if !bytesEqual(prep.Body, c.Call.Body) {
				t.Errorf("prep.Body mutated (len %d → %d)", len(c.Call.Body), len(prep.Body))
			}
		})
	}
}

func checkToolAuthInjection(t *testing.T, spec ToolSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call == nil || (c.Credential == "" && !c.ExpectAuthMissing) {
				t.Skip("no credential to inject")
			}
			prep, err := spec.Connector.PrepareToolRequest(c.Call, c.Credential)
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

func checkToolAuthStripping(t *testing.T, spec ToolSpec) {
	t.Helper()
	hop := []string{"Connection", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding"}
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call == nil {
				t.Skip("no Call fixture")
			}
			call := *c.Call
			call.Header = connruntime.CloneHeader(c.Call.Header)
			call.Header.Set("Authorization", "Bearer inbound-leak-token")
			call.Header.Set("Connection", "keep-alive")
			call.Header.Set("Proxy-Authorization", "Basic leak")
			prep, err := spec.Connector.PrepareToolRequest(&call, c.Credential)
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

func checkToolResponseParsing(t *testing.T, spec ToolSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if len(c.ResponseBody) == 0 {
				t.Skip("no ResponseBody fixture")
			}
			result, err := spec.Connector.ParseToolResponse(c.ResponseBody, c.ResponseType)
			if err != nil {
				t.Fatalf("ParseToolResponse: %v", err)
			}
			if result == nil {
				t.Fatal("ParseToolResponse returned nil without error")
			}
			wantCount := c.ExpectRequestCount
			if wantCount == 0 {
				wantCount = 1
			}
			if result.Usage == nil || result.Usage.RequestCount != wantCount {
				got := 0
				if result.Usage != nil {
					got = result.Usage.RequestCount
				}
				t.Errorf("Usage.RequestCount=%d want %d", got, wantCount)
			}
		})
	}
}

func checkToolPolicyContract(t *testing.T, spec ToolSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call == nil {
				t.Skip("no Call fixture")
			}
			prep, err := spec.Connector.PrepareToolRequest(c.Call, c.Credential)
			if err != nil || prep == nil {
				t.Skipf("prep failed: %v", err)
			}
			if prep.Method == "" {
				t.Error("policy contract: prep.Method empty")
			}
			if prep.URL == "" {
				t.Error("policy contract: prep.URL empty")
			}
			if prep.Header == nil {
				t.Error("policy contract: prep.Header nil")
			}
			if !bytesEqual(prep.Body, c.Call.Body) {
				t.Error("policy contract: prep.Body mutated; policy must see the wire bytes")
			}
		})
	}
}

func checkToolErrorTolerance(t *testing.T, spec ToolSpec) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ParseToolResponse panicked on garbage body: %v", r)
		}
	}()
	if _, err := spec.Connector.ParseToolResponse([]byte(`not json at all`), "application/json"); err != nil {
		t.Logf("ParseToolResponse(garbage) returned error (acceptable): %v", err)
	}
	if _, err := spec.Connector.ParseToolResponse(nil, "application/json"); err != nil {
		t.Logf("ParseToolResponse(nil) returned error (acceptable): %v", err)
	}
}

func checkToolGolden(t *testing.T, spec ToolSpec) {
	t.Helper()
	for _, c := range spec.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			if c.Call != nil {
				if prep, err := spec.Connector.PrepareToolRequest(c.Call, c.Credential); err == nil && prep != nil {
					path := filepath.Join(spec.GoldenDir, c.Name+".request.json")
					assertGoldenJSON(t, path, summarizePrep(prep))
				}
			}
				if len(c.ResponseBody) > 0 {
				if result, err := spec.Connector.ParseToolResponse(c.ResponseBody, c.ResponseType); err == nil && result != nil {
					path := filepath.Join(spec.GoldenDir, c.Name+".result.json")
					assertGoldenJSON(t, path, summarizeToolResult(result))
				}
			}
		})
	}
}

type toolResultSnapshot struct {
	BodyLen      int `json:"body_len"`
	RequestCount int `json:"request_count"`
}

func summarizeToolResult(r *pipeline.Result) toolResultSnapshot {
	snap := toolResultSnapshot{BodyLen: len(r.Body)}
	if r.Usage != nil {
		snap.RequestCount = r.Usage.RequestCount
	}
	return snap
}
