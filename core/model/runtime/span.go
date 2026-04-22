package runtime

import "github.com/kave-io/kave/core/pkg/money"

// SpanRow is the flat database representation of a trace span.
type SpanRow struct {
	ID         string
	ProjectID  string
	EnvID      string
	AgentID    string
	RunID      string
	ActionID   string
	ParentID   *string
	Name       string
	Kind       string
	Source     string
	Connector  string
	StartedAt  int64  // UnixMilli
	EndedAt    *int64 // UnixMilli
	DurationMs int64
	// Input, Output, and Attrs are nullable: nil = not captured, []byte{} = captured as empty
	Input             *[]byte
	Output            *[]byte
	Attrs             *[]byte // typed attributes (AttrVal as JSON)
	Error             *string
	InputTokens       *int
	OutputTokens      *int
	CacheReadTokens   *int
	CacheWriteTokens  *int
	ReasoningTokens   *int
	AudioInputTokens  *int
	AudioOutputTokens *int
	ImageUnits        *int
	RequestCount      *int
	ComputeMs         *int64
	StorageBytes      *int64
	BandwidthBytes    *int64
	Model             *string
	Cost              *money.Amount
	DisplayCost       *DisplayMoney // derived/UI-only; never canonical
	PriceVersion      *string
	PriceSnapshot     *PriceSnapshot
	TraceID           string // OTel trace correlation ID
	RootSpanID        string // root span ID of this trace
	ValidationMeta    []byte // JSON-serialized ValidationMeta
	CreatedAt         int64  // UnixMilli
}

// SpanEnd captures the fields that become available when a span is finalized.
type SpanEnd struct {
	EndedAt    *int64 // UnixMilli
	DurationMs int64
	// Output and Attrs are nullable: nil = not captured, []byte{} = captured as empty
	Output            *[]byte
	Attrs             *[]byte // typed attributes (AttrVal as JSON)
	Error             *string
	InputTokens       *int
	OutputTokens      *int
	CacheReadTokens   *int
	CacheWriteTokens  *int
	ReasoningTokens   *int
	AudioInputTokens  *int
	AudioOutputTokens *int
	ImageUnits        *int
	RequestCount      *int
	ComputeMs         *int64
	StorageBytes      *int64
	BandwidthBytes    *int64
	Model             *string
	Cost              *money.Amount
	DisplayCost       *DisplayMoney // derived/UI-only; never canonical
	PriceVersion      *string
	PriceSnapshot     *PriceSnapshot
	TraceID           string // OTel trace correlation ID
	RootSpanID        string // root span ID of this trace
	ValidationMeta    []byte // JSON-serialized ValidationMeta
}

// SpanFilter narrows QuerySpans queries. Pagination (limit, cursor) is passed
// separately as a store.Page.
type SpanFilter struct {
	ID           string
	RunID        string
	ActionID     string
	TraceID      string
	ProjectID    string
	EnvID        string
	AgentID      string
	Connector    string
	Model        string
	Kind         string
	NamePrefix   string
	HasError     *bool
	FromMs       *int64
	ToMs         *int64
	MinCostMicro *int64
}
