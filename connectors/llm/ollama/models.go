package ollama

import "time"

// ── Core Message & Roles ─────────────────────────────────────────────────────

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role      Role       `json:"role"`
	Content   string     `json:"content"`              // For tool responses, this holds the search/function result
	Images    []string   `json:"images,omitempty"`     // Vision: Array of base64-encoded strings
	ToolCalls []ToolCall `json:"tool_calls,omitempty"` // Assistant requests to use a tool
}

// ── Tooling & Structured Output ──────────────────────────────────────────────

type Tool struct {
	Type     string       `json:"type"` // Currently always "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  JSONSchema `json:"parameters"`
}

type ToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

// JSONSchema supports recursive definitions for complex structured outputs
type JSONSchema struct {
	Type                 string                `json:"type"` // e.g., "object", "string", "array"
	Description          string                `json:"description,omitempty"`
	Properties           map[string]JSONSchema `json:"properties,omitempty"`
	Required             []string              `json:"required,omitempty"`
	Items                *JSONSchema           `json:"items,omitempty"` // For arrays
	Enum                 []any                 `json:"enum,omitempty"`
	AdditionalProperties *bool                 `json:"additionalProperties,omitempty"`
}

// ── Configuration Options ────────────────────────────────────────────────────

type ModelOptions struct {
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`
	RepeatPenalty *float64 `json:"repeat_penalty,omitempty"`
	Seed          *int     `json:"seed,omitempty"`
	NumCtx        *int     `json:"num_ctx,omitempty"`     // Context window size
	NumPredict    *int     `json:"num_predict,omitempty"` // Max tokens to generate
	Stop          []string `json:"stop,omitempty"`
}

type KeepAlive string

const (
	KeepAliveForever KeepAlive = "-1m"
	KeepAliveNone    KeepAlive = "0"
	KeepAlive5m      KeepAlive = "5m" // Ollama default
)

// ── Model Metadata ───────────────────────────────────────────────────────────

type ModelInfo struct {
	Name       string       `json:"name"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

type ModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families,omitempty"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

type RunningModel struct {
	Name      string    `json:"name"`
	Model     string    `json:"model"`
	Size      int64     `json:"size"`
	SizeVRAM  int64     `json:"size_vram"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ProgressResponse struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
}

// ── Helper Functions ──────────────────────────────────────────────────────────

// BoolPtr converts a bool to a *bool pointer.
// Use when assigning to *bool fields (e.g., Stream, Truncate).
func BoolPtr(b bool) *bool {
	return &b
}

// ── Capabilities Implementation Types ────────────────────────────────────────

type WebSearchArgs struct {
	Query string `json:"query"`
}

// ── Embed ─────────────────────────────────────────────────────────────────────

// EmbedPrefix are the recommended prefixes for qwen3-embedding.
type EmbedPrefix string

const (
	PrefixQuery    EmbedPrefix = "search_query: "
	PrefixDocument EmbedPrefix = "search_document: "
	PrefixNone     EmbedPrefix = ""
)

// EmbedOption configures embedding behavior.
type EmbedOption func(*embedConfig)

type embedConfig struct {
	evictAfter bool
}

// EvictAfter unloads the model from VRAM immediately after embedding.
// Use for isolated one-shot calls.
func EvictAfter() EmbedOption {
	return func(c *embedConfig) { c.evictAfter = true }
}
