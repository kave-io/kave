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

	limit, period, err := i.limitForAgent(ctx, action.AgentID)
	if err != nil || limit == nil {
		return action, nil
	}

	spent, err := i.currentSpend(ctx, action.AgentID)
	if err != nil {
		return action, nil
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
	if i == nil || i.store == nil || action == nil || result == nil || result.TokenUsage == nil {
		return nil
	}
	return i.recordUsage(ctx, action, result.TokenUsage)
}

func (i *Interceptor) Name() string { return "budget" }

func (i *Interceptor) limitForAgent(ctx context.Context, agentID string) (*money.Amount, string, error) {
	agent, err := i.store.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, "", err
	}

	period := "monthly"
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
	}
	return limit, period, nil
}

func (i *Interceptor) currentSpend(ctx context.Context, agentID string) (money.Amount, error) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return i.store.SumAgentSpend(ctx, agentID, monthStart.UnixMilli())
}

func (i *Interceptor) recordUsage(ctx context.Context, action *runtime.Action, u *runtime.TokenUsage) error {
	snapshot := (*runtimemodel.PriceSnapshot)(nil)
	if i.prices != nil {
		snapshot = i.prices.Snapshot(action.Connector, u.Model)
	}

	costAmount := cost.Calculate(snapshot,
		u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite,
		u.Reasoning, u.AudioInput, u.AudioOutput, u.ImageUnits,
		0, 0, 0, 0)

	entry := &runtimemodel.BudgetEntry{
		ID:                ids.New("bge"),
		ProjectID:         action.ProjectID,
		EnvID:             action.EnvID,
		AgentID:           action.AgentID,
		RunID:             action.RunID,
		Connector:         action.Connector,
		Model:             u.Model,
		InputTokens:       u.InputTokens,
		OutputTokens:      u.OutputTokens,
		CacheReadTokens:   u.CacheRead,
		CacheWriteTokens:  u.CacheWrite,
		ReasoningTokens:   u.Reasoning,
		AudioInputTokens:  u.AudioInput,
		AudioOutputTokens: u.AudioOutput,
		ImageUnits:        u.ImageUnits,
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

var _ pipeline.Interceptor = (*Interceptor)(nil)
