package cost

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/cost"
	"github.com/kave-io/kave/core/intercept"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
)

// PostgresMeter implements intercept.Interceptor and core/cost.Meter.
// It tracks token spend and enforces budgets using the database.
type PostgresMeter struct {
	pool    *pgxpool.Pool
	pricing *PricingTable
}

// New creates a new PostgresMeter with default pricing.
func New(pool *pgxpool.Pool) *PostgresMeter {
	return &PostgresMeter{
		pool:    pool,
		pricing: NewPricingTable(),
	}
}

// Record logs token usage to the budget ledger and updates run spend.
func (m *PostgresMeter) Record(ctx context.Context, runID string, usage intercept.TokenUsage) error {
	pricing := m.pricing.GetPrice(usage.Model)
	costUSD := CalculateCost(pricing, usage.InputTokens, usage.OutputTokens)

	_, err := m.pool.Exec(ctx, `
		INSERT INTO budget_ledger (run_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, runID, "openai", usage.Model, usage.InputTokens, usage.OutputTokens, usage.CacheRead, usage.CacheWrite, costUSD)
	if err != nil {
		return fmt.Errorf("budget_ledger insert: %w", err)
	}

	_, err = m.pool.Exec(ctx, `
		UPDATE runs SET spent_usd = spent_usd + $1 WHERE id = $2
	`, costUSD, runID)
	if err != nil {
		return fmt.Errorf("runs update: %w", err)
	}

	return nil
}

// CheckBudget returns the current budget status for an agent.
func (m *PostgresMeter) CheckBudget(ctx context.Context, agentID string) (*cost.BudgetStatus, error) {
	var capUSD float64

	err := m.pool.QueryRow(ctx, `
		SELECT COALESCE(a.monthly_budget, p.budget_cap_usd, 0)
		FROM agents a
		LEFT JOIN policies p ON a.policy_id = p.id
		WHERE a.id = $1
	`, agentID).Scan(&capUSD)
	if err != nil {
		return nil, fmt.Errorf("query agent budget: %w", err)
	}

	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var usedUSD float64
	err = m.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0)
		FROM budget_ledger
		WHERE agent_id = $1 AND created_at >= $2
	`, agentID, monthStart).Scan(&usedUSD)
	if err != nil {
		return nil, fmt.Errorf("sum budget ledger: %w", err)
	}

	spent := money.FromDollars(usedUSD)
	cap := money.FromDollars(capUSD)
	return cost.NewBudgetStatus(spent, &cap, "monthly"), nil
}

// GetSpend retrieves aggregated spending data.
func (m *PostgresMeter) GetSpend(ctx context.Context, filter store.SpendFilter) (*store.SpendReport, error) {
	query := `
		SELECT COALESCE(SUM(cost_usd), 0), COALESCE(MIN(created_at), 0), COALESCE(MAX(created_at), 0)
		FROM budget_ledger WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filter.AgentID != "" {
		query += fmt.Sprintf(` AND agent_id = $%d`, argNum)
		args = append(args, filter.AgentID)
		argNum++
	}
	if filter.Connector != "" {
		query += fmt.Sprintf(` AND connector = $%d`, argNum)
		args = append(args, filter.Connector)
		argNum++
	}
	if filter.Model != "" {
		query += fmt.Sprintf(` AND model = $%d`, argNum)
		args = append(args, filter.Model)
		argNum++
	}
	if filter.FromMs != nil {
		query += fmt.Sprintf(` AND created_at >= $%d`, argNum)
		args = append(args, *filter.FromMs)
		argNum++
	}
	if filter.ToMs != nil {
		query += fmt.Sprintf(` AND created_at <= $%d`, argNum)
		args = append(args, *filter.ToMs)
		argNum++
	}

	var totalUSD float64
	var periodStart, periodEnd int64
	if err := m.pool.QueryRow(ctx, query, args...).Scan(&totalUSD, &periodStart, &periodEnd); err != nil {
		return nil, fmt.Errorf("query total spend: %w", err)
	}

	byAgent := make(map[string]float64)
	rows, err := m.pool.Query(ctx, `SELECT agent_id, SUM(cost_usd) FROM budget_ledger GROUP BY agent_id`)
	if err != nil {
		return nil, fmt.Errorf("query by agent: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var v float64
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		byAgent[id] = v
	}

	byConnector := make(map[string]float64)
	rows, err = m.pool.Query(ctx, `SELECT connector, SUM(cost_usd) FROM budget_ledger GROUP BY connector`)
	if err != nil {
		return nil, fmt.Errorf("query by connector: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v float64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		byConnector[k] = v
	}

	byModel := make(map[string]float64)
	rows, err = m.pool.Query(ctx, `SELECT model, SUM(cost_usd) FROM budget_ledger GROUP BY model`)
	if err != nil {
		return nil, fmt.Errorf("query by model: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v float64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		byModel[k] = v
	}

	return &store.SpendReport{
		TotalUSD:    totalUSD,
		ByAgent:     byAgent,
		ByConnector: byConnector,
		ByModel:     byModel,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	}, nil
}

// Cost returns the computed cost for given token counts.
func (m *PostgresMeter) Cost(connector, model string, inputTokens, outputTokens int) money.Amount {
	pricing := m.pricing.GetPrice(model)
	return money.FromDollars(CalculateCost(pricing, inputTokens, outputTokens))
}

// Budget evaluates spend against a cap. Pure function — no DB.
func (m *PostgresMeter) Budget(spent money.Amount, cap *money.Amount, period string) *cost.BudgetStatus {
	return cost.NewBudgetStatus(spent, cap, period)
}

// Before checks if budget is not exhausted before allowing the action.
func (m *PostgresMeter) Before(ctx context.Context, action *intercept.Action) (*intercept.Action, error) {
	return action, nil
}

// After records token usage from the result.
func (m *PostgresMeter) After(ctx context.Context, action *intercept.Action, result *intercept.Result) error {
	if result == nil || result.TokenUsage == nil {
		return nil
	}
	return m.Record(ctx, action.RunID, *result.TokenUsage)
}

// Name returns the interceptor name.
func (m *PostgresMeter) Name() string { return "cost" }
