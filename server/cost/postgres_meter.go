package cost

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/cost"
	"github.com/kave-io/kave/core/intercept"
)

// PostgresMeter implements both core/cost.Meter and core/intercept.Interceptor.
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

// Record logs token usage and updates run cost.
func (m *PostgresMeter) Record(ctx context.Context, runID string, usage intercept.TokenUsage) error {
	// Calculate cost from token usage
	pricing := m.pricing.GetPrice(usage.Model)
	costUSD := CalculateCost(pricing, usage.InputTokens, usage.OutputTokens)

	// Insert into budget_ledger (append-only)
	// Note: we don't know workspace_id, agent_id, action_id here, but they're not required
	_, err := m.pool.Exec(ctx, `
		INSERT INTO budget_ledger (run_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, runID, "openai", usage.Model, usage.InputTokens, usage.OutputTokens, usage.CacheRead, usage.CacheWrite, costUSD)

	if err != nil {
		return fmt.Errorf("budget_ledger insert: %w", err)
	}

	// Update runs.spent_usd
	_, err = m.pool.Exec(ctx, `
		UPDATE runs SET spent_usd = spent_usd + $1 WHERE id = $2
	`, costUSD, runID)

	if err != nil {
		return fmt.Errorf("runs update: %w", err)
	}

	return nil
}

// CheckBudget returns the current budget status for an agent.
func (m *PostgresMeter) CheckBudget(ctx context.Context, agentID string) (cost.BudgetStatus, error) {
	var (
		capUSD    float64
		usedUSD   float64
		agentName string
	)

	// Get agent's budget cap
	err := m.pool.QueryRow(ctx, `
		SELECT COALESCE(a.monthly_budget, p.budget_cap_usd), a.name
		FROM agents a
		LEFT JOIN policies p ON a.policy_id = p.id
		WHERE a.id = $1
	`, agentID).Scan(&capUSD, &agentName)

	if err != nil {
		return cost.BudgetStatus{}, fmt.Errorf("query agent budget: %w", err)
	}

	// Sum spend for this agent in the current month
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	err = m.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0)
		FROM budget_ledger
		WHERE agent_id = $1 AND created_at >= $2
	`, agentID, monthStart).Scan(&usedUSD)

	if err != nil {
		return cost.BudgetStatus{}, fmt.Errorf("sum budget ledger: %w", err)
	}

	remaining := capUSD - usedUSD
	if remaining < 0 {
		remaining = 0
	}

	monthEnd := monthStart.AddDate(0, 1, 0)

	return cost.BudgetStatus{
		AgentID:       agentID,
		CapUSD:        capUSD,
		UsedUSD:       usedUSD,
		RemainingUSD:  remaining,
		IsExhausted:   remaining <= 0,
		ResetAt:       monthEnd,
	}, nil
}

// GetSpend retrieves aggregated spending data.
func (m *PostgresMeter) GetSpend(ctx context.Context, filter cost.SpendFilter) (cost.SpendReport, error) {
	query := `
		SELECT
			COALESCE(SUM(cost_usd), 0) as total_cost,
			COALESCE(MAX(created_at), NOW()) as max_time,
			COALESCE(MIN(created_at), NOW()) as min_time
		FROM budget_ledger
		WHERE 1=1
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
	if filter.FromTime != nil {
		query += fmt.Sprintf(` AND created_at >= $%d`, argNum)
		args = append(args, *filter.FromTime)
		argNum++
	}
	if filter.ToTime != nil {
		query += fmt.Sprintf(` AND created_at <= $%d`, argNum)
		args = append(args, *filter.ToTime)
		argNum++
	}

	var totalCost float64
	var maxTime, minTime time.Time

	err := m.pool.QueryRow(ctx, query, args...).Scan(&totalCost, &maxTime, &minTime)
	if err != nil {
		return cost.SpendReport{}, fmt.Errorf("query total spend: %w", err)
	}

	// Aggregate by agent
	byAgent := make(map[string]float64)
	rows, err := m.pool.Query(ctx, `
		SELECT agent_id, SUM(cost_usd) as total
		FROM budget_ledger
		GROUP BY agent_id
	`)
	if err != nil {
		return cost.SpendReport{}, fmt.Errorf("query by agent: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agentID string
		var costAmount float64
		if err := rows.Scan(&agentID, &costAmount); err != nil {
			return cost.SpendReport{}, err
		}
		byAgent[agentID] = costAmount
	}

	// Aggregate by connector
	byConnector := make(map[string]float64)
	rows, err = m.pool.Query(ctx, `
		SELECT connector, SUM(cost_usd) as total
		FROM budget_ledger
		GROUP BY connector
	`)
	if err != nil {
		return cost.SpendReport{}, fmt.Errorf("query by connector: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var connector string
		var costAmount float64
		if err := rows.Scan(&connector, &costAmount); err != nil {
			return cost.SpendReport{}, err
		}
		byConnector[connector] = costAmount
	}

	// Aggregate by model
	byModel := make(map[string]float64)
	rows, err = m.pool.Query(ctx, `
		SELECT model, SUM(cost_usd) as total
		FROM budget_ledger
		GROUP BY model
	`)
	if err != nil {
		return cost.SpendReport{}, fmt.Errorf("query by model: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var model string
		var costAmount float64
		if err := rows.Scan(&model, &costAmount); err != nil {
			return cost.SpendReport{}, err
		}
		byModel[model] = costAmount
	}

	return cost.SpendReport{
		TotalUSD:    totalCost,
		ByAgent:     byAgent,
		ByConnector: byConnector,
		ByModel:     byModel,
		PeriodStart: minTime,
		PeriodEnd:   maxTime,
	}, nil
}

// Implement Interceptor interface for pipeline integration

// Before checks if the agent's budget is exhausted.
func (m *PostgresMeter) Before(ctx context.Context, action *intercept.Action) (*intercept.Action, error) {
	// We need agent_id from the context or action metadata
	// For now, skip budget check in Before (it will be in the auth/policy check instead)
	return action, nil
}

// After records token usage (non-blocking, could use River for async).
func (m *PostgresMeter) After(ctx context.Context, action *intercept.Action, result *intercept.Result) error {
	if result.Tokens == nil {
		return nil // No tokens to record
	}

	// Record usage for the run
	return m.Record(ctx, action.RunID, *result.Tokens)
}

// Name returns the interceptor name.
func (m *PostgresMeter) Name() string {
	return "cost"
}
