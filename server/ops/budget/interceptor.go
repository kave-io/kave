package budget

import (
	"context"
	"errors"
	"fmt"
	"time"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/ops/cost"
	"github.com/tidwall/gjson"
)

var ErrBudgetExceeded = errors.New("gateway budget exceeded")

// ExceededError carries the spend and cap used for the block decision.
type ExceededError struct {
	Spent   money.Amount
	Limit   money.Amount
	Period  string
	Subject string
}

func (e *ExceededError) Error() string {
	if e == nil {
		return ErrBudgetExceeded.Error()
	}
	return fmt.Sprintf("budget exceeded: spent=%s limit=%s period=%s", e.Spent.String(), e.Limit.String(), e.Period)
}

func (e *ExceededError) Unwrap() error { return ErrBudgetExceeded }

// Interceptor enforces spend caps and records successful spend.
type Interceptor struct {
	store  store.AppStore
	prices *cost.Service
}

func New(app store.AppStore, prices *cost.Service) *Interceptor {
	return &Interceptor{store: app, prices: prices}
}

func (i *Interceptor) Before(ctx context.Context, action *runtime.Action) (*runtime.Action, error) {
	if i == nil || i.store == nil || action == nil || action.AgentID == "" {
		return action, nil
	}

	limit, period, behavior, err := i.limitForAgent(ctx, action.AgentID)
	if err != nil || limit == nil {
		return action, nil
	}
	if behavior == "warn" {
		return action, nil
	}

	spent, err := i.currentSpend(ctx, action.AgentID)
	if err != nil {
		return action, nil
	}
	if behavior == "reserve" || behavior == "reserve_cap" {
		reserve := i.estimateReservation(action)
		if reserve <= 0 || spent+reserve <= *limit {
			return action, nil
		}
		_ = i.recordBlock(ctx, action, "budget reserve exceeded", period)
		action.Status = runtime.StatusBlocked
		action.Outcome = &runtime.Outcome{Code: "gateway.budget_exceeded", Message: "budget reserve exceeded", Reason: "budget reserve exceeded"}
		return nil, &ExceededError{Spent: spent + reserve, Limit: *limit, Period: period, Subject: action.AgentID}
	}
	if spent < *limit {
		return action, nil
	}

	_ = i.recordBlock(ctx, action, "budget exceeded", period)

	action.Status = runtime.StatusBlocked
	action.Outcome = &runtime.Outcome{
		Code:    "gateway.budget_exceeded",
		Message: "budget exceeded",
		Reason:  "budget exceeded",
	}
	return nil, &ExceededError{
		Spent:   spent,
		Limit:   *limit,
		Period:  period,
		Subject: action.AgentID,
	}
}

func (i *Interceptor) After(ctx context.Context, action *runtime.Action, result *pipeline.Result) error {
	if i == nil || i.store == nil || action == nil || result == nil {
		return nil
	}
	usage := result.Usage
	if usage == nil && result.TokenUsage != nil {
		usage = &runtime.Usage{Tokens: result.TokenUsage, RequestCount: 1}
	}
	if usage == nil {
		return nil
	}
	return i.recordUsage(ctx, action, usage)
}

func (i *Interceptor) Name() string { return "budget" }

func (i *Interceptor) limitForAgent(ctx context.Context, agentID string) (*money.Amount, string, string, error) {
	agent, err := i.store.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, "", "", err
	}

	period := "monthly"
	behavior := "block"
	var limit *money.Amount
	if agent != nil && agent.MonthlyBudget != nil {
		limit = agent.MonthlyBudget
	}
	if pol, err := i.store.GetAgentPolicy(ctx, agentID); err == nil && pol != nil {
		if pol.BudgetCap > 0 {
			v := pol.BudgetCap
			limit = &v
		}
		if pol.BudgetPeriod != "" {
			period = pol.BudgetPeriod
		}
		if pol.BudgetBehavior != "" {
			behavior = pol.BudgetBehavior
		}
	}
	return limit, period, behavior, nil
}

func (i *Interceptor) currentSpend(ctx context.Context, agentID string) (money.Amount, error) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return i.store.SumAgentSpend(ctx, agentID, monthStart.UnixMilli())
}

func (i *Interceptor) recordUsage(ctx context.Context, action *runtime.Action, usage *runtime.Usage) error {
	u := usage.Tokens
	snapshot := (*runtimemodel.PriceSnapshot)(nil)
	model := ""
	if u != nil {
		model = u.Model
	}
	if i.prices != nil && u != nil {
		snapshot = i.prices.Snapshot(action.Connector, u.Model)
	}

	costAmount := cost.Calculate(snapshot,
		tokenInt(u, func(t *runtime.TokenUsage) int { return t.InputTokens }),
		tokenInt(u, func(t *runtime.TokenUsage) int { return t.OutputTokens }),
		tokenInt(u, func(t *runtime.TokenUsage) int { return t.CacheRead }),
		tokenInt(u, func(t *runtime.TokenUsage) int { return t.CacheWrite }),
		tokenInt(u, func(t *runtime.TokenUsage) int { return t.Reasoning }),
		tokenInt(u, func(t *runtime.TokenUsage) int { return t.AudioInput }),
		tokenInt(u, func(t *runtime.TokenUsage) int { return t.AudioOutput }),
		tokenInt(u, func(t *runtime.TokenUsage) int { return t.ImageUnits }),
		usage.RequestCount, usage.ComputeMs, usage.StorageBytes, usage.BandwidthBytes)

	entry := &runtimemodel.BudgetEntry{
		ID:                ids.New("bge"),
		ProjectID:         action.ProjectID,
		EnvID:             action.EnvID,
		AgentID:           action.AgentID,
		RunID:             action.RunID,
		Connector:         action.Connector,
		Model:             model,
		InputTokens:       tokenInt(u, func(t *runtime.TokenUsage) int { return t.InputTokens }),
		OutputTokens:      tokenInt(u, func(t *runtime.TokenUsage) int { return t.OutputTokens }),
		CacheReadTokens:   tokenInt(u, func(t *runtime.TokenUsage) int { return t.CacheRead }),
		CacheWriteTokens:  tokenInt(u, func(t *runtime.TokenUsage) int { return t.CacheWrite }),
		ReasoningTokens:   tokenInt(u, func(t *runtime.TokenUsage) int { return t.Reasoning }),
		AudioInputTokens:  tokenInt(u, func(t *runtime.TokenUsage) int { return t.AudioInput }),
		AudioOutputTokens: tokenInt(u, func(t *runtime.TokenUsage) int { return t.AudioOutput }),
		ImageUnits:        tokenInt(u, func(t *runtime.TokenUsage) int { return t.ImageUnits }),
		RequestCount:      usage.RequestCount,
		ComputeMs:         usage.ComputeMs,
		StorageBytes:      usage.StorageBytes,
		BandwidthBytes:    usage.BandwidthBytes,
		Cost:              costAmount,
		CreatedAt:         int64(timex.Now()),
	}
	if snapshot != nil {
		entry.PriceVersion = snapshot.Version
		entry.PriceSnapshot = snapshot
	}
	if action.ID != "" {
		entry.ActionID = &action.ID
	}

	if err := i.store.InsertBudgetEntry(ctx, entry); err != nil {
		return err
	}
	if entry.RunID != "" {
		return i.store.AddRunSpend(ctx, entry.RunID, costAmount)
	}
	return nil
}

func (i *Interceptor) estimateReservation(action *runtime.Action) money.Amount {
	if i == nil || i.prices == nil || action == nil || action.Input == nil {
		return 0
	}
	body := *action.Input
	model := gjson.GetBytes(body, "model").String()
	if model == "" {
		return 0
	}
	maxOutput := firstPositive(
		int(gjson.GetBytes(body, "max_tokens").Int()),
		int(gjson.GetBytes(body, "max_completion_tokens").Int()),
		int(gjson.GetBytes(body, "max_output_tokens").Int()),
	)
	if maxOutput <= 0 {
		return 0
	}
	snapshot := i.prices.Snapshot(action.Connector, model)
	if snapshot == nil {
		return 0
	}
	return cost.Calculate(snapshot, 0, maxOutput, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0)
}

func tokenInt(u *runtime.TokenUsage, f func(*runtime.TokenUsage) int) int {
	if u == nil {
		return 0
	}
	return f(u)
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// recordBlock persists a zero-cost ledger row for a block decision.
// Plain-typed fields replace the previous untyped metadata map.
func (i *Interceptor) recordBlock(ctx context.Context, action *runtime.Action, reason, period string) error {
	entry := &runtimemodel.BudgetEntry{
		ID:          ids.New("bge"),
		ProjectID:   action.ProjectID,
		EnvID:       action.EnvID,
		AgentID:     action.AgentID,
		RunID:       action.RunID,
		Connector:   action.Connector,
		Cost:        0,
		Blocked:     true,
		BlockReason: reason,
		BlockPeriod: period,
		CreatedAt:   int64(timex.Now()),
	}
	if action.ID != "" {
		entry.ActionID = &action.ID
	}
	return i.store.InsertBudgetEntry(ctx, entry)
}

var _ pipeline.Stage = (*Interceptor)(nil)
