package trace

import (
	"context"
	"encoding/json"

	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
)

// attrKind discriminates the type of a typed attribute value.
type attrKind uint8

const (
	attrString attrKind = iota
	attrInt
	attrFloat
	attrBool
)

// AttrVal is a typed span attribute value. Only valid types: string, int64, float64, bool.
// Matches OTel attribute semantics without the full SDK dependency.
type AttrVal struct {
	kind attrKind
	s    string
	n    int64
	f    float64
	b    bool
}

func StringAttr(v string) AttrVal  { return AttrVal{kind: attrString, s: v} }
func IntAttr(v int64) AttrVal      { return AttrVal{kind: attrInt, n: v} }
func FloatAttr(v float64) AttrVal  { return AttrVal{kind: attrFloat, f: v} }
func BoolAttr(v bool) AttrVal      { return AttrVal{kind: attrBool, b: v} }

func (a AttrVal) IsString() bool  { return a.kind == attrString }
func (a AttrVal) IsInt() bool     { return a.kind == attrInt }
func (a AttrVal) IsFloat() bool   { return a.kind == attrFloat }
func (a AttrVal) IsBool() bool    { return a.kind == attrBool }
func (a AttrVal) Str() string     { return a.s }
func (a AttrVal) Int() int64      { return a.n }
func (a AttrVal) Float() float64  { return a.f }
func (a AttrVal) Bool() bool      { return a.b }

// MarshalJSON encodes AttrVal as {"type":"string","value":"..."} for storage.
func (a AttrVal) MarshalJSON() ([]byte, error) {
	switch a.kind {
	case attrString:
		return json.Marshal(map[string]interface{}{"type": "string", "value": a.s})
	case attrInt:
		return json.Marshal(map[string]interface{}{"type": "int", "value": a.n})
	case attrFloat:
		return json.Marshal(map[string]interface{}{"type": "float", "value": a.f})
	case attrBool:
		return json.Marshal(map[string]interface{}{"type": "bool", "value": a.b})
	default:
		return []byte("null"), nil
	}
}

// UnmarshalJSON decodes AttrVal from {"type":"...","value":"..."} format.
func (a *AttrVal) UnmarshalJSON(data []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if t, ok := m["type"].(string); ok {
		switch t {
		case "string":
			if v, ok := m["value"].(string); ok {
				a.kind = attrString
				a.s = v
			}
		case "int":
			if v, ok := m["value"].(float64); ok {
				a.kind = attrInt
				a.n = int64(v)
			}
		case "float":
			if v, ok := m["value"].(float64); ok {
				a.kind = attrFloat
				a.f = v
			}
		case "bool":
			if v, ok := m["value"].(bool); ok {
				a.kind = attrBool
				a.b = v
			}
		}
	}
	return nil
}

// SpanKind classifies what sort of execution the span represents.
type SpanKind string

const (
	SpanKindAction         SpanKind = "action"
	SpanKindObservedAction SpanKind = "observed_action"
	SpanKindImport         SpanKind = "import"
)

// SpanSource explains how Kave learned about the span.
type SpanSource string

const (
	SourceIntercept  SpanSource = "intercept"
	SourceReport     SpanSource = "report"
	SourceOTELImport SpanSource = "otel_import"
)

// ValidationMeta carries validation execution details for a span.
type ValidationMeta struct {
	ValidatorName    string
	ValidatorVersion string
	RuleVersion      string
	EnforcementMode  string // "block" | "warn" | "audit"
	DurationMs       int64
	Valid            bool
	ViolationCount   int
}

// Span is the immutable final record of one execution.
// Stores may open it first and close/finalize it later, but the completed span
// itself is never updated after finalization.
type Span struct {
	ID               string
	ProjectID        string
	EnvID            string
	AgentID          string
	RunID            string
	ActionID         string
	ParentID         *string
	Name             string
	Kind             SpanKind
	Source           SpanSource
	Connector        string
	Model            *string
	StartedAt        timex.MS
	EndedAt          timex.MS
	DurationMS       int64
	InputTokens      *int
	OutputTokens     *int
	Cost             *money.Amount
	Error            *string
	Attrs            map[string]AttrVal
	TraceID          string  // OTel trace correlation ID (not nullable)
	RootSpanID       string  // Root span ID of this trace (not nullable)
	ValidationMeta   *ValidationMeta
}

// ComputeDuration returns EndedAt - StartedAt in milliseconds.
func (s *Span) ComputeDuration() int64 {
	if s.StartedAt.IsZero() || s.EndedAt.IsZero() {
		return 0
	}
	return int64(s.EndedAt - s.StartedAt)
}

// Tracer writes spans to a span store.
type Tracer interface {
	Record(ctx context.Context, span *Span) error
}
