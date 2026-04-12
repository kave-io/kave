package cost

import (
	"context"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	coreCost "github.com/kave-io/kave/core/ops/cost"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
	"github.com/kave-io/kave/core/store"
)

// CostMeter implements pipeline.Interceptor and coreCost.Meter.
type CostMeter struct {
	app    store.AppStore
	prices *Service
}

func New(app store.AppStore, prices *Service) *CostMeter {
	return &CostMeter{app: app, prices: prices}
}

// Cost implements coreCost.Pricer.
func (m *CostMeter) Cost(connector, model string, usage *coreCost.CostUsage) money.Amount {
	if usage == nil {
		return 0
	}
	return Calculate(m.prices.Snapshot(connector, model),
		usage.Input, usage.Output, usage.CacheRead, usage.CacheWrite,
		usage.Reasoning, usage.AudioInput, usage.AudioOutput, usage.ImageUnits,
		usage.RequestCount, usage.ComputeMs, usage.StorageBytes, usage.BandwidthBytes)
}

// Budget implements coreCost.BudgetEvaluator.
func (m *CostMeter) Budget(spent money.Amount, p *policy.CostPolicy) *coreCost.BudgetStatus {
	if p == nil {
		return coreCost.NewBudgetStatus(spent, nil, "run")
	}
	return coreCost.NewBudgetStatus(spent, p.BudgetCap, p.BudgetPeriod)
}

// Record logs token usage to the budget ledger and updates run spend.
func (m *CostMeter) Record(ctx context.Context, action *runtime.Action, u *runtime.TokenUsage) error {
	if action.AgentID == "" || action.ProjectID == "" || u == nil {
		return nil
	}

	snapshot := m.prices.Snapshot(action.Connector, u.Model)
	costAmount := Calculate(snapshot,
		u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite,
		u.Reasoning, u.AudioInput, u.AudioOutput, u.ImageUnits,
		0, 0, 0, 0)

	entry := &runtimemodel.BudgetEntry{
		ID:                uuid.NewString(),
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
		Metadata:          map[string]any{},
		CreatedAt:         int64(timex.Now()),
	}
	if snapshot != nil {
		entry.PriceVersion = snapshot.Version
		entry.PriceSnapshot = snapshot
	}
	if action.ID != "" {
		entry.ActionID = &action.ID
	}

	if err := m.app.InsertBudgetEntry(ctx, entry); err != nil {
		return err
	}
	if entry.RunID != "" {
		return m.app.AddRunSpend(ctx, entry.RunID, costAmount)
	}
	return nil
}

// CheckBudget returns the current budget status for an agent.
func (m *CostMeter) CheckBudget(ctx context.Context, agentID string) (*coreCost.BudgetStatus, error) {
	agent, err := m.app.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	usedAmount, err := m.app.SumAgentSpend(ctx, agentID, monthStart.UnixMilli())
	if err != nil {
		return nil, err
	}

	var cap *money.Amount
	if agent != nil && agent.MonthlyBudget != nil {
		cap = agent.MonthlyBudget
	}

	return coreCost.NewBudgetStatus(usedAmount, cap, "monthly"), nil
}

// GetSpend retrieves aggregated spending data.
func (m *CostMeter) GetSpend(ctx context.Context, filter *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	return m.app.GetSpendReport(ctx, filter)
}

// Before is a no-op; budget blocking is done inline by the gateway.
func (m *CostMeter) Before(ctx context.Context, action *runtime.Action) (*runtime.Action, error) {
	return action, nil
}

// After records token usage from the result.
func (m *CostMeter) After(ctx context.Context, action *runtime.Action, result *pipeline.Result) error {
	if result == nil || result.TokenUsage == nil {
		return nil
	}
	return m.Record(ctx, action, result.TokenUsage)
}

func (m *CostMeter) Name() string { return "cost" }

var _ pipeline.Interceptor = (*CostMeter)(nil)
var _ coreCost.Meter = (*CostMeter)(nil)
