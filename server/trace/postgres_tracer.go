package trace

import (
	"context"
	"strings"
	"time"

	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/store"
)

// Tracer implements intercept.Interceptor.
// It records all agent actions as spans in the configured span store.
type Tracer struct {
	spans store.SpanStore
}

func New(spans store.SpanStore) *Tracer {
	return &Tracer{spans: spans}
}

// Before stamps the action start time.
func (t *Tracer) Before(ctx context.Context, action *intercept.Action) (*intercept.Action, error) {
	action.StartedAt = timex.Now()
	return action, nil
}

// After records the span with timing, tokens, cost.
func (t *Tracer) After(ctx context.Context, action *intercept.Action, result *intercept.Result) error {
	action.EndedAt = timex.Now()

	now := time.Now().UnixMilli()
	endedAt := int64(action.EndedAt)

	row := &store.SpanRow{
		ID:         action.ID,
		RunID:      action.RunID,
		ActionID:   action.ID,
		StartedAt:  int64(action.StartedAt),
		EndedAt:    &endedAt,
		DurationMs: timex.Since(action.StartedAt),
		Error:      action.Error,
		CreatedAt:  now,
	}

	if result != nil {
		action.Output = result.Body

		if u := result.TokenUsage; u != nil {
			row.InputTokens = &u.InputTokens
			row.OutputTokens = &u.OutputTokens
			if u.CacheRead > 0 {
				row.CacheReadTokens = &u.CacheRead
			}
			if u.CacheWrite > 0 {
				row.CacheWriteTokens = &u.CacheWrite
			}
			if u.Model != "" {
				row.Model = &u.Model
				if c := costUSD(u.Model, u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite); c != nil {
					row.CostUSD = c
				}
			}
		}
	}

	return t.spans.WriteSpan(ctx, row)
}

func (t *Tracer) Name() string { return "tracer" }

// ── pricing ───────────────────────────────────────────────────────────────────

type pricing struct {
	substr  string
	inPerM  float64 // input tokens per million USD
	outPerM float64 // output tokens per million USD
	cacheReadPerM  float64
	cacheWritePerM float64
}

var modelPricing = []pricing{
	// Anthropic Claude 4.x
	{"claude-opus-4",   15.00, 75.00, 1.50, 18.75},
	{"claude-sonnet-4",  3.00, 15.00, 0.30,  3.75},
	{"claude-haiku-4",   0.80,  4.00, 0.08,  1.00},
	// Anthropic Claude 3.x
	{"claude-3-5-sonnet", 3.00, 15.00, 0.30, 3.75},
	{"claude-3-5-haiku",  0.80,  4.00, 0.08, 1.00},
	{"claude-3-opus",    15.00, 75.00, 1.50, 18.75},
	{"claude-3-haiku",    0.25,  1.25, 0.03, 0.30},
	// OpenAI
	{"gpt-4o-mini",  0.15,  0.60, 0.075, 0.0},
	{"gpt-4o",       2.50, 10.00, 1.25,  0.0},
	{"gpt-4-turbo", 10.00, 30.00, 0.0,   0.0},
	{"o1-mini",      1.10,  4.40, 0.55,  0.0},
	{"o1",          15.00, 60.00, 7.50,  0.0},
	// Google Gemini
	{"gemini-2.0-flash", 0.10, 0.40, 0.025, 0.0},
	{"gemini-1.5-pro",   1.25, 5.00, 0.3125, 0.0},
	{"gemini-1.5-flash", 0.075, 0.30, 0.01875, 0.0},
	// Groq
	{"llama-3.3", 0.06, 0.06, 0.0, 0.0},
	{"llama-3.1", 0.05, 0.05, 0.0, 0.0},
	// Mistral
	{"mistral-large", 2.00, 6.00, 0.0, 0.0},
	{"mistral-small", 0.20, 0.60, 0.0, 0.0},
}

// costUSD returns cost in dollars, or nil if the model is unknown (e.g. local Ollama).
func costUSD(model string, in, out, cacheRead, cacheWrite int) *float64 {
	lower := strings.ToLower(model)
	for _, p := range modelPricing {
		if strings.Contains(lower, p.substr) {
			v := (float64(in)*p.inPerM +
				float64(out)*p.outPerM +
				float64(cacheRead)*p.cacheReadPerM +
				float64(cacheWrite)*p.cacheWritePerM) / 1_000_000
			return &v
		}
	}
	return nil
}
