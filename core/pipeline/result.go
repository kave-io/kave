package pipeline

import "github.com/kave-io/kave/core/runtime"

// Result is the handler's return payload. Transport-agnostic — no HTTP status codes
// or headers. Works for proxy responses, MCP JSON-RPC replies, and streaming chunks.
type Result struct {
	Body              []byte
	TokenUsage        *runtime.TokenUsage // nil for non-LLM actions
	Usage             *runtime.Usage      // nil when there is nothing to meter
	ObservedSpans     []runtime.ObservedSpan
	ProviderRequestID string
}
