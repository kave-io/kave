package cost

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kave-io/kave/core/cost"
	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/pkg/timex"
	"github.com/kave-io/kave/core/store"
)

// CostMeter implements intercept.Interceptor and core/cost.Meter.
// It tracks token spend and persists the exact price snapshot used.
type CostMeter struct {
	app    store.AppStore
	prices *Service
}

// New creates a new cost meter backed by the configured AppStore and pricing book.
func New(app store.AppStore, prices *Service) *CostMeter {
	return &CostMeter{
		app:    app,
		prices: prices,
	}
}

// Record logs token usage to the budget ledger and updates run spend.
func (m *CostMeter) Record(ctx context.Context, action *intercept.Action, usage intercept.TokenUsage) error {
	if action.AgentID == "" || action.WorkspaceID == "" {
		return nil
	}

	snapshot := m.prices.Snapshot(action.Connector, usage.Model)
	costAmount := Calculate(snapshot, usage.InputTokens, usage.OutputTokens, usage.CacheRead, usage.CacheWrite)
	entry := &store.BudgetEntry{
		ID:               uuid.NewString(),
		WorkspaceID:      action.WorkspaceID,
		AgentID:          action.AgentID,
		Connector:        action.Connector,
		Model:            usage.Model,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheRead,
		CacheWriteTokens: usage.CacheWrite,
		CostUSD:          costAmount.Dollars(),
		Metadata:         map[string]any{},
		CreatedAt:        int64(timex.Now()),
	}
	if snapshot != nil {
		entry.PriceVersion = snapshot.Version
		entry.PriceSnapshot = snapshot
	}
	entry.ActionID = &action.ID

	if action.RunID != "" {
		run, err := m.app.GetRunByID(ctx, action.RunID)
		if err == nil && run != nil {
			entry.RunID = run.ID
		}
	}

	if err := m.app.InsertBudgetEntry(ctx, entry); err != nil {
		return err
	}
	if entry.RunID != "" {
		return m.app.AddRunSpend(ctx, entry.RunID, entry.CostUSD)
	}
	return nil
}

// CheckBudget returns the current budget status for an agent.
func (m *CostMeter) CheckBudget(ctx context.Context, agentID string) (*cost.BudgetStatus, error) {
	agent, err := m.app.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	usedUSD, err := m.app.SumAgentSpend(ctx, agentID, int64(timex.From(monthStart)))
	if err != nil {
		return nil, err
	}

	var cap *money.Amount
	if agent != nil && agent.MonthlyBudget != nil {
		value := money.FromDollars(*agent.MonthlyBudget)
		cap = &value
	} else if policy, err := m.app.GetAgentPolicy(ctx, agentID); err == nil && policy != nil && policy.BudgetCapUSD > 0 {
		value := money.FromDollars(policy.BudgetCapUSD)
		cap = &value
	}

	spent := money.FromDollars(usedUSD)
	return cost.NewBudgetStatus(spent, cap, "monthly"), nil
}

// GetSpend retrieves aggregated spending data.
func (m *CostMeter) GetSpend(ctx context.Context, filter store.SpendFilter) (*store.SpendReport, error) {
	return m.app.GetSpendReport(ctx, &filter)
}

// Cost returns the computed cost for given token counts.
func (m *CostMeter) Cost(connector, model string, inputTokens, outputTokens int) money.Amount {
	return Calculate(m.prices.Snapshot(connector, model), inputTokens, outputTokens, 0, 0)
}

// Budget evaluates spend against a cap. Pure function — no DB.
func (m *CostMeter) Budget(spent money.Amount, cap *money.Amount, period string) *cost.BudgetStatus {
	return cost.NewBudgetStatus(spent, cap, period)
}

// Before checks if budget is not exhausted before allowing the action.
func (m *CostMeter) Before(ctx context.Context, action *intercept.Action) (*intercept.Action, error) {
	return action, nil
}

// After records token usage from the result.
func (m *CostMeter) After(ctx context.Context, action *intercept.Action, result *intercept.Result) error {
	if result == nil || result.TokenUsage == nil {
		return nil
	}
	return m.Record(ctx, action, *result.TokenUsage)
}

// Name returns the interceptor name.
func (m *CostMeter) Name() string { return "cost" }
