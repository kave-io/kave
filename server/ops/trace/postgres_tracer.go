package trace

import (
	"context"
	"encoding/json"

	"github.com/kave-io/kave/core/bus"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/runtime"
	coreStore "github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/ops/cost"
)

type SpanStoreResolver interface {
	SpanStore(ctx context.Context, agentID string) (coreStore.SpanStore, error)
}

// Tracer implements pipeline.Interceptor and routes spans to the configured store.
type Tracer struct {
	spans  SpanStoreResolver
	prices *cost.Service
	bus    *bus.Bus // may be nil
}

func New(spans SpanStoreResolver, prices *cost.Service, b *bus.Bus) *Tracer {
	return &Tracer{spans: spans, prices: prices, bus: b}
}

// Before stamps the action start time.
func (t *Tracer) Before(ctx context.Context, action *runtime.Action) (*runtime.Action, error) {
	action.StartedAt = timex.Now()
	return action, nil
}

// After records the span with timing, tokens, cost.
func (t *Tracer) After(ctx context.Context, action *runtime.Action, result *pipeline.Result) error {
	now := timex.Now()
	endedAtMS := int64(now)
	startedAtMS := int64(action.StartedAt)
	durationMs := int64(timex.Since(action.StartedAt))

	spanStore, err := t.spans.SpanStore(ctx, action.AgentID)
	if err != nil {
		return err
	}

	spanID := action.SpanID
	if spanID == "" {
		spanID = ids.SpanID()
	}
	traceCtx := runtime.TraceFrom(ctx)
	rootSpanID := traceCtx.RootSpanID
	if rootSpanID == "" {
		rootSpanID = spanID
	}
	if action.ParentID == "" {
		rootSpanID = spanID
	}

	row := &runtimemodel.SpanRow{
		ID:         spanID,
		ProjectID:  action.ProjectID,
		EnvID:      action.EnvID,
		AgentID:    action.AgentID,
		RunID:      action.RunID,
		ActionID:   action.ID,
		ParentID:   stringPtr(action.ParentID),
		Name:       action.Connector + "." + action.Method,
		Kind:       string(action.Type),
		Source:     string(runtime.ActionSourceIntercepted),
		Connector:  action.Connector,
		StartedAt:  startedAtMS,
		EndedAt:    &endedAtMS,
		DurationMs: durationMs,
		Error:      action.Error,
		TraceID:    action.TraceID,
		RootSpanID: rootSpanID,
		CreatedAt:  endedAtMS,
	}

	if result != nil {
		if len(result.Body) > 0 {
			action.Output = &result.Body
			row.Output = &result.Body
		}

		if u := result.TokenUsage; u != nil {
			row.InputTokens = &u.InputTokens
			row.OutputTokens = &u.OutputTokens
			ptrIfPos := func(v int) *int {
				if v > 0 {
					return &v
				}
				return nil
			}
			row.CacheReadTokens = ptrIfPos(u.CacheRead)
			row.CacheWriteTokens = ptrIfPos(u.CacheWrite)
			row.ReasoningTokens = ptrIfPos(u.Reasoning)
			row.AudioInputTokens = ptrIfPos(u.AudioInput)
			row.AudioOutputTokens = ptrIfPos(u.AudioOutput)
			row.ImageUnits = ptrIfPos(u.ImageUnits)

			if u.Model != "" {
				row.Model = &u.Model
				snapshot := t.prices.Snapshot(action.Connector, u.Model)
				if snapshot != nil {
					c := cost.Calculate(snapshot,
						u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite,
						u.Reasoning, u.AudioInput, u.AudioOutput, u.ImageUnits,
						0, 0, 0, 0)
					row.Cost = &c
					row.PriceVersion = &snapshot.Version
					row.PriceSnapshot = snapshot
				}
			}
		}
	}

	if err := spanStore.OpenSpan(ctx, row); err != nil {
		return err
	}
	if t.bus != nil {
		raw, err := json.Marshal(row)
		if err == nil {
			t.bus.Publish(bus.Event{
				Kind:      "span.completed",
				ProjectID: action.ProjectID,
				EnvID:     action.EnvID,
				RunID:     action.RunID,
				AgentID:   action.AgentID,
				SpanID:    spanID,
				At:        endedAtMS,
				Payload:   raw,
			})
		}
	}
	return nil
}

func (t *Tracer) Name() string { return "tracer" }

var _ pipeline.Interceptor = (*Tracer)(nil)

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
