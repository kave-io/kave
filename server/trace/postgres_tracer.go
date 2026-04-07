package trace

import (
	"context"
	"strings"

	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/store"
	coretrace "github.com/kave-io/kave/core/trace"
)

// modelPricing maps model name substrings to [inputPerM, outputPerM] in USD.
// Match is case-insensitive substring — first match wins.
var modelPricing = []struct {
	substr string
	inPerM float64
	outPerM float64
}{
	// Anthropic Claude 4.x
	{"claude-opus-4",    15.00, 75.00},
	{"claude-sonnet-4",   3.00, 15.00},
	{"claude-haiku-4",    0.80,  4.00},
	// Anthropic Claude 3.x
	{"claude-3-5-sonnet", 3.00, 15.00},
	{"claude-3-5-haiku",  0.80,  4.00},
	{"claude-3-opus",    15.00, 75.00},
	{"claude-3-haiku",    0.25,  1.25},
	// OpenAI
	{"gpt-4o-mini",       0.15,  0.60},
	{"gpt-4o",            2.50, 10.00},
	{"gpt-4-turbo",      10.00, 30.00},
	{"gpt-3.5-turbo",     0.50,  1.50},
	{"o1-mini",           1.10,  4.40},
	{"o1",               15.00, 60.00},
	// Google Gemini
	{"gemini-2.0-flash",  0.10,  0.40},
	{"gemini-1.5-pro",    1.25,  5.00},
	{"gemini-1.5-flash",  0.075, 0.30},
	// Groq (very cheap inference)
	{"llama-3.3",         0.06,  0.06},
	{"llama-3.1",         0.05,  0.05},
	// Mistral
	{"mistral-large",     2.00,  6.00},
	{"mistral-small",     0.20,  0.60},
}

// costUSD calculates the cost for a model call. Returns nil if model is unknown
// (e.g. local Ollama models that run free).
func costUSD(model string, inputTokens, outputTokens int) *money.Amount {
	lower := strings.ToLower(model)
	for _, p := range modelPricing {
		if strings.Contains(lower, p.substr) {
			dollars := (float64(inputTokens)*p.inPerM + float64(outputTokens)*p.outPerM) / 1_000_000
			v := money.FromDollars(dollars)
			return &v
		}
	}
	return nil
}

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
				span.Cost = costUSD(u.Model, u.InputTokens, u.OutputTokens)
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
