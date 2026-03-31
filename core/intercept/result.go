package intercept

// Result is the handler's return payload. Transport-agnostic — no HTTP status codes
// or headers. Works for proxy responses, MCP JSON-RPC replies, and streaming chunks.
type Result struct {
	Body []byte
}
