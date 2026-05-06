package runtime

import (
	"fmt"
	"net/http"

	"github.com/kave-io/kave/core/pipeline"
	coreruntime "github.com/kave-io/kave/core/runtime"
)

// Request is the inbound request shape connectors can inspect without depending
// on server packages.
type Request struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// LLMCall is the normalized request a framework exposes to the server.
type LLMCall struct {
	Provider     string
	Method       string
	UpstreamPath string
	RawQuery     string
	Header       http.Header
	Body         []byte
	Action       *coreruntime.Action
}

// PreparedRequest is the outbound HTTP request an LLM connector wants the
// server transport to execute.
type PreparedRequest struct {
	Method string
	URL    string
	Header http.Header
	Body   []byte
}

// ToolCall is an observed tool invocation discovered in LLM wire data. It is
// not blockable unless the tool call also crosses a Kave tool connector route.
type ToolCall struct {
	ID        string
	Name      string
	Type      string
	Arguments string
	Index     int
}

// ToolCallRequest is the normalized shape for an intercepted tool call.
type ToolCallRequest struct {
	Connector    string
	Method       string
	HTTPMethod   string
	UpstreamPath string
	RawQuery     string
	Header       http.Header
	Body         []byte
	Action       *coreruntime.Action
}

// LLMFramework parses framework-specific inbound traffic into a normalized LLM call.
type LLMFramework interface {
	Name() string
	ParseLLMRequest(req *Request) (*LLMCall, error)
}

// LLMConnector owns provider-specific request preparation and response parsing.
type LLMConnector interface {
	Name() string
	PrepareRequest(call *LLMCall, credential string) (*PreparedRequest, error)
	ParseResponse(body []byte, contentType string) (*pipeline.Result, error)
	RequiresAuth() bool // true if upstream provider requires credentials; false for local/free providers
}

// ToolConnector owns a tool/API upstream translation. It does not execute the
// call; the server gateway is still the only HTTP egress point.
type ToolConnector interface {
	Name() string
	ParseToolRequest(req *Request) (*ToolCallRequest, error)
	PrepareToolRequest(call *ToolCallRequest, credential string) (*PreparedRequest, error)
	ParseToolResponse(body []byte, contentType string) (*pipeline.Result, error)
	RequiresAuth() bool
}

// CloneHeader copies a header map for safe mutation.
func CloneHeader(src http.Header) http.Header {
	if src == nil {
		return make(http.Header)
	}

	dst := make(http.Header, len(src))
	for k, vs := range src {
		copied := make([]string, len(vs))
		copy(copied, vs)
		dst[k] = copied
	}
	return dst
}

// RequireProvider returns the selected provider connector or an error.
func RequireProvider(connectors map[string]LLMConnector, name string) (LLMConnector, error) {
	connector, ok := connectors[name]
	if !ok {
		return nil, fmt.Errorf("unknown llm provider: %q", name)
	}
	return connector, nil
}
