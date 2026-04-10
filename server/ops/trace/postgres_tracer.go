package trace

import (
	"context"

	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/ops/cost"
)

type SpanStoreResolver interface {
	SpanStore(ctx context.Context, agentID string) (store.SpanStore, error)
}

// Tracer implements intercept.Interceptor and routes spans to the configured store.
type Tracer struct {
	spans  SpanStoreResolver
	prices *cost.Service
}

func New(spans SpanStoreResolver, prices *cost.Service) *Tracer {
	return &Tracer{spans: spans, prices: prices}
}

// Before stamps the action start time.
func (t *Tracer) Before(ctx context.Context, action *intercept.Action) (*intercept.Action, error) {
	action.StartedAt = timex.Now()
	return action, nil
}

// After records the span with timing, tokens, cost.
func (t *Tracer) After(ctx context.Context, action *intercept.Action, result *intercept.Result) error {
	action.EndedAt = timex.Now()

	spanStore, err := t.spans.SpanStore(ctx, action.AgentID)
	if err != nil {
		return err
	}

	now := int64(timex.Now())
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
				if snapshot := t.prices.Snapshot(action.Connector, u.Model); snapshot != nil {
					costUSD := cost.Calculate(snapshot, u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite).Dollars()
					row.CostUSD = &costUSD
					row.PriceVersion = &snapshot.Version
					row.PriceSnapshot = snapshot
				}
			}
		}
	}

	return spanStore.WriteSpan(ctx, row)
}

func (t *Tracer) Name() string { return "tracer" }
