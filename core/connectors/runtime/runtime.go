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
