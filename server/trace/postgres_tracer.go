package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/core/trace"
)

// Tracer implements both core/trace.Tracer and core/intercept.Interceptor.
// It records all agent actions as spans in the configured span store.
type Tracer struct {
	spans store.SpanStore
}

// New creates a new Tracer.
func New(spans store.SpanStore) *Tracer {
	return &Tracer{spans: spans}
}

// StartSpan inserts a new span record and returns it with span ID in context.
func (t *Tracer) StartSpan(ctx context.Context, action *intercept.Action) (trace.Span, context.Context) {
	span := trace.Span{
		ID:        action.ID,
		RunID:     action.RunID,
		ActionID:  action.ID,
		Name:      fmt.Sprintf("%s.%s", action.Connector, action.Method),
		StartedAt: time.Now(),
		Input:     action.Input,
		Tags:      map[string]string{},
	}

	// Write span to store
	inputJSON, _ := json.Marshal(action.Input)
	spanRow := &store.SpanRow{
		ID:        span.ID,
		RunID:     span.RunID,
		ActionID:  span.ActionID,
		Name:      span.Name,
		StartedAt: span.StartedAt.UnixMilli(),
		Input:     inputJSON,
	}
	_ = t.spans.WriteSpan(ctx, spanRow)

	// Store span in context for retrieval in After hook
	ctx = context.WithValue(ctx, contextKeySpan, &span)
	return span, ctx
}

// EndSpan updates the span with output, duration, and token usage.
func (t *Tracer) EndSpan(ctx context.Context, span trace.Span, result *intercept.Result) error {
	now := time.Now()
	durationMs := now.Sub(span.StartedAt).Milliseconds()

	outputJSON := []byte("{}")
	if result.Output != nil {
		if b, err := json.Marshal(result.Output); err == nil {
			outputJSON = b
		}
	}

	var errStr *string
	if result.Error != nil {
		s := result.Error.Error()
		errStr = &s
	}

	spanRow := &store.SpanRow{
		ID:         span.ID,
		RunID:      span.RunID,
		ActionID:   span.ActionID,
		EndedAt:    ptrInt64(now.UnixMilli()),
		DurationMs: durationMs,
		Output:     outputJSON,
		Error:      errStr,
	}

	// Add token usage if present
	if result.Tokens != nil {
		spanRow.InputTokens = ptrInt(result.Tokens.InputTokens)
		spanRow.OutputTokens = ptrInt(result.Tokens.OutputTokens)
		spanRow.CacheReadTokens = ptrInt(result.Tokens.CacheRead)
		spanRow.CacheWriteTokens = ptrInt(result.Tokens.CacheWrite)
		spanRow.Model = ptrString(result.Tokens.Model)
		spanRow.CostUSD = ptrFloat64(result.Tokens.CostUSD)
	}

	return t.spans.UpdateSpan(ctx, spanRow)
}

// GetSpan retrieves a span by ID.
func (t *Tracer) GetSpan(ctx context.Context, spanID string) (*trace.Span, error) {
	row, err := t.spans.GetSpan(ctx, spanID)
	if err != nil || row == nil {
		return nil, err
	}

	span := &trace.Span{
		ID:        row.ID,
		RunID:     row.RunID,
		ActionID:  row.ActionID,
		ParentID:  ptrStringValue(row.ParentID),
		Name:      row.Name,
		StartedAt: time.UnixMilli(row.StartedAt),
		EndedAt:   ptrTime(row.EndedAt),
		DurationMs: row.DurationMs,
		Error:     row.Error,
	}

	if row.Input != nil {
		_ = json.Unmarshal(row.Input, &span.Input)
	}
	if span.Input == nil {
		span.Input = make(map[string]any)
	}

	if row.Output != nil {
		_ = json.Unmarshal(row.Output, &span.Output)
	}
	if span.Output == nil {
		span.Output = make(map[string]any)
	}

	if row.Tags != nil {
		_ = json.Unmarshal(row.Tags, &span.Tags)
	}
	if span.Tags == nil {
		span.Tags = make(map[string]string)
	}

	if row.InputTokens != nil {
		span.TokenUsage = &intercept.TokenUsage{
			InputTokens:  *row.InputTokens,
			OutputTokens: ptrIntValue(row.OutputTokens),
			CacheRead:    ptrIntValue(row.CacheReadTokens),
			CacheWrite:   ptrIntValue(row.CacheWriteTokens),
			Model:        ptrStringValue(row.Model),
			CostUSD:      ptrFloat64Value(row.CostUSD),
		}
	}

	return span, nil
}

// QuerySpans retrieves spans matching a filter.
func (t *Tracer) QuerySpans(ctx context.Context, filter trace.SpanFilter) ([]trace.Span, error) {
	storeFilter := &store.SpanFilter{
		RunID:    filter.RunID,
		ActionID: filter.ActionID,
		HasError: filter.HasError,
		Limit:    filter.Limit,
	}
	if filter.FromTime != nil {
		ms := filter.FromTime.UnixMilli()
		storeFilter.FromMs = &ms
	}
	if filter.ToTime != nil {
		ms := filter.ToTime.UnixMilli()
		storeFilter.ToMs = &ms
	}

	rows, err := t.spans.QuerySpans(ctx, storeFilter)
	if err != nil {
		return nil, err
	}

	var spans []trace.Span
	for _, row := range rows {
		span := trace.Span{
			ID:         row.ID,
			RunID:      row.RunID,
			ActionID:   row.ActionID,
			ParentID:   ptrStringValue(row.ParentID),
			Name:       row.Name,
			StartedAt:  time.UnixMilli(row.StartedAt),
			EndedAt:    ptrTime(row.EndedAt),
			DurationMs: row.DurationMs,
			Error:      row.Error,
		}

		if row.Input != nil {
			_ = json.Unmarshal(row.Input, &span.Input)
		}
		if span.Input == nil {
			span.Input = make(map[string]any)
		}

		if row.Output != nil {
			_ = json.Unmarshal(row.Output, &span.Output)
		}
		if span.Output == nil {
			span.Output = make(map[string]any)
		}

		if row.Tags != nil {
			_ = json.Unmarshal(row.Tags, &span.Tags)
		}
		if span.Tags == nil {
			span.Tags = make(map[string]string)
		}

		if row.InputTokens != nil {
			span.TokenUsage = &intercept.TokenUsage{
				InputTokens:  *row.InputTokens,
				OutputTokens: ptrIntValue(row.OutputTokens),
				CacheRead:    ptrIntValue(row.CacheReadTokens),
				CacheWrite:   ptrIntValue(row.CacheWriteTokens),
				Model:        ptrStringValue(row.Model),
				CostUSD:      ptrFloat64Value(row.CostUSD),
			}
		}

		spans = append(spans, span)
	}

	return spans, nil
}

// Before starts a span when an action begins.
func (t *Tracer) Before(ctx context.Context, action *intercept.Action) (*intercept.Action, error) {
	_, _ = t.StartSpan(ctx, action)
	return action, nil
}

// After ends the span when an action completes.
func (t *Tracer) After(ctx context.Context, action *intercept.Action, result *intercept.Result) error {
	span := trace.Span{
		ID:        action.ID,
		RunID:     action.RunID,
		ActionID:  action.ID,
		StartedAt: time.Now(), // Placeholder
	}
	return t.EndSpan(ctx, span, result)
}

// Name returns the interceptor name.
func (t *Tracer) Name() string {
	return "tracer"
}

// Helper functions for pointer conversions
func ptrInt(i int) *int                   { return &i }
func ptrInt64(i int64) *int64             { return &i }
func ptrFloat64(f float64) *float64       { return &f }
func ptrString(s string) *string          { return &s }
func ptrIntValue(p *int) int              { if p == nil { return 0 }; return *p }
func ptrFloat64Value(p *float64) float64  { if p == nil { return 0 }; return *p }
func ptrStringValue(p *string) string     { if p == nil { return "" }; return *p }
func ptrTime(ms *int64) *time.Time        { if ms == nil { return nil }; t := time.UnixMilli(*ms); return &t }

// Context key for storing span
var contextKeySpan = struct{}{}
