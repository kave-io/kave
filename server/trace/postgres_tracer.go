package trace

import (
	"context"

	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/store"
	coretrace "github.com/kave-io/kave/core/trace"
)

// Tracer implements intercept.Interceptor and core/trace.Tracer.
// It records all agent actions as spans in the configured span store.
type Tracer struct {
	spans store.SpanStore
}

// New creates a new Tracer.
func New(spans store.SpanStore) *Tracer {
	return &Tracer{spans: spans}
}

// Before stamps the action start time.
func (t *Tracer) Before(ctx context.Context, action *intercept.Action) (*intercept.Action, error) {
	action.StartedAt = timex.Now()
	return action, nil
}

// After records the span with timing, output, and token usage.
func (t *Tracer) After(ctx context.Context, action *intercept.Action, result *intercept.Result) error {
	action.EndedAt = timex.Now()

	span := &coretrace.Span{
		ID:         action.ID,
		RunID:      action.RunID,
		ActionID:   action.ID,
		Connector:  action.Connector,
		StartedAt:  action.StartedAt,
		EndedAt:    action.EndedAt,
		DurationMS: timex.Since(action.StartedAt),
		Error:      action.Error,
		Source:     coretrace.SourceIntercept,
	}

	if result != nil {
		action.Output = result.Body

		if result.TokenUsage != nil {
			u := result.TokenUsage
			span.InputTokens = &u.InputTokens
			span.OutputTokens = &u.OutputTokens
			if u.Model != "" {
				span.Model = &u.Model
			}
		}
	}

	return t.Record(ctx, span)
}

// Record writes a span to the span store.
func (t *Tracer) Record(ctx context.Context, span *coretrace.Span) error {
	row := &store.SpanRow{
		ID:           span.ID,
		RunID:        span.RunID,
		ActionID:     span.ActionID,
		StartedAt:    int64(span.StartedAt),
		DurationMs:   span.DurationMS,
		Error:        span.Error,
		InputTokens:  span.InputTokens,
		OutputTokens: span.OutputTokens,
		Model:        span.Model,
	}

	if !span.EndedAt.IsZero() {
		v := int64(span.EndedAt)
		row.EndedAt = &v
	}

	if span.Cost != nil {
		v := span.Cost.Dollars()
		row.CostUSD = &v
	}

	return t.spans.WriteSpan(ctx, row)
}

// Name returns the interceptor name.
func (t *Tracer) Name() string { return "tracer" }
