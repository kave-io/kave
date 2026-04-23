package anthropic

// ── Messages ──────────────────────────────────────────────────────────────────

type MessagesRequest struct {
	Model         string    `json:"model"`
	Messages      []Message `json:"messages"`
	MaxTokens     int       `json:"max_tokens"`
	System        *string   `json:"system,omitempty"`
	Stream        *bool     `json:"stream,omitempty"`
	Temperature   *float64  `json:"temperature,omitempty"`
	TopP          *float64  `json:"top_p,omitempty"`
	TopK          *int      `json:"top_k,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type MessagesResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ── Streaming ─────────────────────────────────────────────────────────────────

type streamEvent struct {
	Type string `json:"type"`
}

type contentBlockDeltaEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"delta"`
}

type StreamChunk struct {
	Content string
	Done    bool
	Error   error
}

// ── Errors ────────────────────────────────────────────────────────────────────

type ErrorResponse struct {
	Type  string   `json:"type"`
	Error APIError `json:"error"`
}

type APIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
