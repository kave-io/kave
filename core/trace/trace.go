package trace

import (
	"context"

	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
)

// Valid Source values for a Span.
const (
	SourceIntercept  = "intercept"   // created by Kave's intercept pipeline
	SourceReport     = "report"      // reported by an SDK after the fact
	SourceOTELImport = "otel_import" // imported from OpenTelemetry
)

// Span is the immutable record of one execution. Written once, never updated.
// Stored in SpanStore — separate from Action/Event lifecycle records.
type Span struct {
	ID           string
	RunID        string
	ActionID     string            // references Action.ID or Event.ID
	Connector    string
	Model        *string           // "gpt-4o", nil for non-llm
	StartedAt    timex.MS
	EndedAt      timex.MS
	DurationMS   int64    // elapsed time, not a timestamp
	InputTokens  *int
	OutputTokens *int
	Cost         *money.Amount
	Error        *string
	Tags         map[string]string
	Source       string            // "intercept" | "report" | "otel_import"
}

// ComputeDuration returns EndedAt - StartedAt in milliseconds.
// Returns 0 if either timestamp is unset.
func (s *Span) ComputeDuration() int64 {
	if s.StartedAt.IsZero() || s.EndedAt.IsZero() {
		return 0
	}
	return int64(s.EndedAt - s.StartedAt)
}

// Tracer writes spans to a span store.
// Implementations (SQLite, DuckDB, ClickHouse) live in server/.
type Tracer interface {
	Record(ctx context.Context, span *Span) error
}
