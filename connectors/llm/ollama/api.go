package ollama

import (
	"time"
)

// ---------------------------------------------------------------------------
// Generate API (/api/generate)
// ---------------------------------------------------------------------------

type GenerateRequest struct {
	Model       string        `json:"model"`
	Prompt      string        `json:"prompt"`
	Suffix      string        `json:"suffix,omitempty"`
	System      string        `json:"system,omitempty"`
	Template    string        `json:"template,omitempty"`
	Context     []int         `json:"context,omitempty"`
	Stream      *bool         `json:"stream,omitempty"` // Use pointer to distinguish between false and unset (default true)
	Raw         bool          `json:"raw,omitempty"`
	Format      any           `json:"format,omitempty"`     // Can be "json" string or a JSONSchema struct
	KeepAlive   any           `json:"keep_alive,omitempty"` // Duration string (e.g., "5m") or seconds int
	Images      []string      `json:"images,omitempty"`     // Base64 encoded images (Vision)
	Options     *ModelOptions `json:"options,omitempty"`
	Think       any           `json:"think,omitempty"`        // bool or string ("high", "medium", "low") for reasoning models
	Logprobs    bool          `json:"logprobs,omitempty"`     // Return token log probabilities
	TopLogprobs int           `json:"top_logprobs,omitempty"` // Top tokens to return probabilities for
}

type GenerateResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Response           string    `json:"response"` // For streaming, this is the delta content
	Done               bool      `json:"done"`
	DoneReason         string    `json:"done_reason,omitempty"`
	Context            []int     `json:"context,omitempty"`
	TotalDuration      int64     `json:"total_duration,omitempty"` // Nanoseconds
	LoadDuration       int64     `json:"load_duration,omitempty"`  // Nanoseconds
	PromptEvalCount    int       `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64     `json:"prompt_eval_duration,omitempty"` // Nanoseconds
	EvalCount          int       `json:"eval_count,omitempty"`
	EvalDuration       int64     `json:"eval_duration,omitempty"` // Nanoseconds
	Thinking           string    `json:"thinking,omitempty"`      // Thinking output from reasoning models
	Logprobs           []any     `json:"logprobs,omitempty"`      // Token probability information
}

// ---------------------------------------------------------------------------
// Chat API (/api/chat)
// ---------------------------------------------------------------------------

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []Message     `json:"messages"`
	Tools       []Tool        `json:"tools,omitempty"`
	Format      any           `json:"format,omitempty"`
	Stream      *bool         `json:"stream,omitempty"`
	KeepAlive   any           `json:"keep_alive,omitempty"`
	Options     *ModelOptions `json:"options,omitempty"`
	Think       any           `json:"think,omitempty"`
	Logprobs    bool          `json:"logprobs,omitempty"`     // Return token log probabilities
	TopLogprobs int           `json:"top_logprobs,omitempty"` // Top tokens to return probabilities for
}

type ChatResponse struct {
	Model      string    `json:"model"`
	CreatedAt  time.Time `json:"created_at"`
	Message    Message   `json:"message"` // The assistant's message or delta
	Done       bool      `json:"done"`
	DoneReason string    `json:"done_reason,omitempty"`
	// Performance Metrics (Nanoseconds)
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
	Thinking           string `json:"thinking,omitempty"` // Thinking output from reasoning models
	Logprobs           []any  `json:"logprobs,omitempty"` // Token probability information
}

// StreamChunk wraps streaming responses with delta content and error handling.
// Kave-specific type: extracts content deltas from Chat/Generate responses
// and provides an Error field for stream-level errors.
type StreamChunk struct {
	Content string // Delta content (from Message.Content or Response field)
	Done    bool   // Whether the stream is complete
	Error   error  // Stream-level error (nil if successful)
}

// ---------------------------------------------------------------------------
// Embeddings API (/api/embed)
// ---------------------------------------------------------------------------

type EmbedRequest struct {
	Model      string        `json:"model"`
	Input      any           `json:"input"` // string or []string
	Truncate   *bool         `json:"truncate,omitempty"`
	Dimensions int           `json:"dimensions,omitempty"` // Output embedding dimensions
	KeepAlive  any           `json:"keep_alive,omitempty"`
	Options    *ModelOptions `json:"options,omitempty"`
}

type EmbedResponse struct {
	Model           string      `json:"model"`
	Embeddings      [][]float32 `json:"embeddings"`
	TotalDuration   int64       `json:"total_duration,omitempty"`
	LoadDuration    int64       `json:"load_duration,omitempty"`
	PromptEvalCount int         `json:"prompt_eval_count,omitempty"`
}

// ---------------------------------------------------------------------------
// Management & Listing APIs
// ---------------------------------------------------------------------------

type PullRequest struct {
	Model    string `json:"model"`
	Insecure bool   `json:"insecure,omitempty"`
	Stream   *bool  `json:"stream,omitempty"`
}

type ShowResponse struct {
	License    string       `json:"license,omitempty"`
	Modelfile  string       `json:"modelfile,omitempty"`
	Parameters string       `json:"parameters,omitempty"`
	Template   string       `json:"template,omitempty"`
	System     string       `json:"system,omitempty"`
	Details    ModelDetails `json:"details"`
	Messages   []Message    `json:"messages,omitempty"`
}

type ListResponse struct {
	Models []ModelInfo `json:"models"`
}

type ListRunningResponse struct {
	Models []RunningModel `json:"models"`
}
