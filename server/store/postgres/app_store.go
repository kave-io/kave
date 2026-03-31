package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/infra/postgres"
)

// PostgresAppStore implements store.AppStore using Postgres via pgxpool.
type PostgresAppStore struct {
	pool *pgxpool.Pool
}

// New creates a new Postgres app store.
func New(pool *pgxpool.Pool) *PostgresAppStore {
	return &PostgresAppStore{pool: pool}
}

// Close closes the connection pool.
func (p *PostgresAppStore) Close() error {
	p.pool.Close()
	return nil
}

// Migrate runs pending migrations.
func (p *PostgresAppStore) Migrate(ctx context.Context) error {
	return postgres.Migrate(ctx, p.pool)
}

// CreateWorkspace creates a new workspace.
func (p *PostgresAppStore) CreateWorkspace(ctx context.Context, w *store.Workspace) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO workspaces (id, name, slug, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, w.ID, w.Name, w.Slug, w.Description, toTime(w.CreatedAt), toTime(w.UpdatedAt))
	return err
}

// GetWorkspace retrieves a workspace by ID.
func (p *PostgresAppStore) GetWorkspace(ctx context.Context, id string) (*store.Workspace, error) {
	var w store.Workspace
	var createdAt, updatedAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, name, slug, description, created_at, updated_at
		FROM workspaces WHERE id = $1
	`, id).Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	w.CreatedAt = createdAt.UnixMilli()
	w.UpdatedAt = updatedAt.UnixMilli()
	return &w, nil
}

// CreateAgent creates a new agent.
func (p *PostgresAppStore) CreateAgent(ctx context.Context, a *store.Agent) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO agents (id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, a.ID, a.WorkspaceID, a.Name, a.Description, a.PolicyID, a.MonthlyBudget, metaJSON, toTime(a.CreatedAt), toTime(a.UpdatedAt))
	return err
}

// GetAgentByID retrieves an agent by ID.
func (p *PostgresAppStore) GetAgentByID(ctx context.Context, id string) (*store.Agent, error) {
	return p.getAgent(ctx, `
		SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at
		FROM agents WHERE id = $1
	`, id)
}

// GetAgentByName retrieves an agent by workspace and name.
func (p *PostgresAppStore) GetAgentByName(ctx context.Context, workspaceID, name string) (*store.Agent, error) {
	return p.getAgent(ctx, `
		SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at
		FROM agents WHERE workspace_id = $1 AND name = $2
	`, workspaceID, name)
}

func (p *PostgresAppStore) getAgent(ctx context.Context, query string, args ...any) (*store.Agent, error) {
	var a store.Agent
	var metaJSON []byte
	var createdAt, updatedAt time.Time
	err := p.pool.QueryRow(ctx, query, args...).Scan(
		&a.ID, &a.WorkspaceID, &a.Name, &a.Description, &a.PolicyID, &a.MonthlyBudget, &metaJSON, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
		a.Metadata = make(map[string]any)
	}
	a.CreatedAt = createdAt.UnixMilli()
	a.UpdatedAt = updatedAt.UnixMilli()
	return &a, nil
}

// UpdateAgent updates an agent.
func (p *PostgresAppStore) UpdateAgent(ctx context.Context, id string, update *store.AgentUpdate) error {
	now := time.Now()
	metaJSON, _ := json.Marshal(update.Metadata)
	_, err := p.pool.Exec(ctx, `
		UPDATE agents
		SET description = COALESCE($1, description),
		    policy_id = COALESCE($2, policy_id),
		    monthly_budget = COALESCE($3, monthly_budget),
		    metadata = COALESCE($4, metadata),
		    updated_at = $5
		WHERE id = $6
	`, update.Description, update.PolicyID, update.MonthlyBudget, metaJSON, now, id)
	return err
}

// ListAgents retrieves all agents in a workspace.
func (p *PostgresAppStore) ListAgents(ctx context.Context, workspaceID string) ([]*store.Agent, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at
		FROM agents WHERE workspace_id = $1 ORDER BY name
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*store.Agent
	for rows.Next() {
		var a store.Agent
		var metaJSON []byte
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Description, &a.PolicyID, &a.MonthlyBudget, &metaJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
			a.Metadata = make(map[string]any)
		}
		a.CreatedAt = createdAt.UnixMilli()
		a.UpdatedAt = updatedAt.UnixMilli()
		agents = append(agents, &a)
	}
	return agents, rows.Err()
}

// CreatePolicy creates a new policy.
func (p *PostgresAppStore) CreatePolicy(ctx context.Context, pol *store.Policy) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO policies (id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, pol.ID, pol.WorkspaceID, pol.Name, pol.Description, pol.AllowedConnectors, pol.AllowedMethods, pol.BudgetCapUSD, pol.Config, toTime(pol.CreatedAt), toTime(pol.UpdatedAt))
	return err
}

// GetPolicy retrieves a policy by ID.
func (p *PostgresAppStore) GetPolicy(ctx context.Context, id string) (*store.Policy, error) {
	var pol store.Policy
	var createdAt, updatedAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at
		FROM policies WHERE id = $1
	`, id).Scan(&pol.ID, &pol.WorkspaceID, &pol.Name, &pol.Description, &pol.AllowedConnectors, &pol.AllowedMethods, &pol.BudgetCapUSD, &pol.Config, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	pol.CreatedAt = createdAt.UnixMilli()
	pol.UpdatedAt = updatedAt.UnixMilli()
	return &pol, nil
}

// GetAgentPolicy retrieves the policy associated with an agent.
func (p *PostgresAppStore) GetAgentPolicy(ctx context.Context, agentID string) (*store.Policy, error) {
	var policyID *string
	err := p.pool.QueryRow(ctx, `SELECT policy_id FROM agents WHERE id = $1`, agentID).Scan(&policyID)
	if err != nil || policyID == nil {
		return nil, err
	}
	return p.GetPolicy(ctx, *policyID)
}

// CreateRun creates a new run.
func (p *PostgresAppStore) CreateRun(ctx context.Context, r *store.Run) error {
	metaJSON, _ := json.Marshal(r.Metadata)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO runs (id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, r.ID, r.WorkspaceID, r.AgentID, r.PolicyID, r.Name, r.Status, r.BudgetCapUSD, r.SpentUSD, metaJSON, r.ErrorMessage, toTime(r.StartedAt), ptrToTime(r.EndedAt), toTime(r.CreatedAt), toTime(r.UpdatedAt))
	return err
}

// GetRunByID retrieves a run by ID.
func (p *PostgresAppStore) GetRunByID(ctx context.Context, id string) (*store.Run, error) {
	var r store.Run
	var metaJSON []byte
	var createdAt, updatedAt time.Time
	var endedAt *time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at
		FROM runs WHERE id = $1
	`, id).Scan(&r.ID, &r.WorkspaceID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &r.BudgetCapUSD, &r.SpentUSD, &metaJSON, &r.ErrorMessage, &r.StartedAt, &endedAt, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(metaJSON, &r.Metadata); err != nil {
		r.Metadata = make(map[string]any)
	}
	if endedAt != nil {
		ms := endedAt.UnixMilli()
		r.EndedAt = &ms
	}
	r.StartedAt = createdAt.UnixMilli()
	r.CreatedAt = createdAt.UnixMilli()
	r.UpdatedAt = updatedAt.UnixMilli()
	return &r, nil
}

// UpdateRun updates a run.
func (p *PostgresAppStore) UpdateRun(ctx context.Context, id string, update *store.RunUpdate) error {
	now := time.Now()
	metaJSON, _ := json.Marshal(update.Metadata)
	_, err := p.pool.Exec(ctx, `
		UPDATE runs
		SET status = COALESCE($1, status),
		    spent_usd = COALESCE($2, spent_usd),
		    error_message = COALESCE($3, error_message),
		    ended_at = COALESCE($4, ended_at),
		    metadata = COALESCE($5, metadata),
		    updated_at = $6
		WHERE id = $7
	`, update.Status, update.SpentUSD, update.ErrorMessage, ptrToTime(update.EndedAt), metaJSON, now, id)
	return err
}

// ListRuns retrieves runs matching a filter.
func (p *PostgresAppStore) ListRuns(ctx context.Context, filter *store.RunFilter) ([]*store.Run, error) {
	query := `
		SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at
		FROM runs WHERE 1=1
	`
	var args []any
	argNum := 1

	if filter.WorkspaceID != "" {
		query += fmt.Sprintf(` AND workspace_id = $%d`, argNum)
		args = append(args, filter.WorkspaceID)
		argNum++
	}
	if filter.AgentID != "" {
		query += fmt.Sprintf(` AND agent_id = $%d`, argNum)
		args = append(args, filter.AgentID)
		argNum++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(` AND status = $%d`, argNum)
		args = append(args, filter.Status)
		argNum++
	}
	if filter.FromMs != nil {
		query += fmt.Sprintf(` AND started_at >= $%d`, argNum)
		args = append(args, toTime(*filter.FromMs))
		argNum++
	}
	if filter.ToMs != nil {
		query += fmt.Sprintf(` AND started_at <= $%d`, argNum)
		args = append(args, toTime(*filter.ToMs))
		argNum++
	}

	query += ` ORDER BY started_at DESC`
	if filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT $%d`, argNum)
		args = append(args, filter.Limit)
	}

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*store.Run
	for rows.Next() {
		var r store.Run
		var metaJSON []byte
		var createdAt, updatedAt time.Time
		var endedAt *time.Time
		if err := rows.Scan(&r.ID, &r.WorkspaceID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &r.BudgetCapUSD, &r.SpentUSD, &metaJSON, &r.ErrorMessage, &r.StartedAt, &endedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metaJSON, &r.Metadata); err != nil {
			r.Metadata = make(map[string]any)
		}
		if endedAt != nil {
			ms := endedAt.UnixMilli()
			r.EndedAt = &ms
		}
		r.CreatedAt = createdAt.UnixMilli()
		r.UpdatedAt = updatedAt.UnixMilli()
		runs = append(runs, &r)
	}
	return runs, rows.Err()
}

// InsertBudgetEntry inserts a budget ledger entry.
func (p *PostgresAppStore) InsertBudgetEntry(ctx context.Context, entry *store.BudgetEntry) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO budget_ledger (id, workspace_id, agent_id, run_id, action_id, span_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, entry.ID, entry.WorkspaceID, entry.AgentID, entry.RunID, entry.ActionID, entry.SpanID, entry.Connector, entry.Model, entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens, entry.CostUSD, entry.Metadata, toTime(entry.CreatedAt))
	return err
}

// AddRunSpend updates the spent_usd on a run.
func (p *PostgresAppStore) AddRunSpend(ctx context.Context, runID string, costUSD float64) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE runs SET spent_usd = spent_usd + $1 WHERE id = $2
	`, costUSD, runID)
	return err
}

// SumAgentSpend sums the cost for an agent since a given timestamp.
func (p *PostgresAppStore) SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (float64, error) {
	var total float64
	err := p.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM budget_ledger WHERE agent_id = $1 AND created_at >= $2
	`, agentID, toTime(sinceMs)).Scan(&total)
	return total, err
}

// GetSpendReport retrieves aggregated spend data.
func (p *PostgresAppStore) GetSpendReport(ctx context.Context, filter *store.SpendFilter) (*store.SpendReport, error) {
	query := `SELECT COALESCE(SUM(cost_usd), 0), MAX(created_at), MIN(created_at) FROM budget_ledger WHERE 1=1`
	var args []any
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
		args = append(args, toTime(*filter.FromMs))
		argNum++
	}
	if filter.ToMs != nil {
		query += fmt.Sprintf(` AND created_at <= $%d`, argNum)
		args = append(args, toTime(*filter.ToMs))
		argNum++
	}

	var total float64
	var maxTime, minTime *time.Time
	err := p.pool.QueryRow(ctx, query, args...).Scan(&total, &maxTime, &minTime)
	if err != nil {
		return nil, err
	}

	report := &store.SpendReport{
		TotalUSD:    total,
		ByAgent:     make(map[string]float64),
		ByConnector: make(map[string]float64),
		ByModel:     make(map[string]float64),
	}

	if minTime != nil {
		report.PeriodStart = minTime.UnixMilli()
	}
	if maxTime != nil {
		report.PeriodEnd = maxTime.UnixMilli()
	}

	// Aggregate by agent
	rows, _ := p.pool.Query(ctx, `SELECT agent_id, SUM(cost_usd) FROM budget_ledger GROUP BY agent_id`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var cost float64
			if err := rows.Scan(&id, &cost); err == nil {
				report.ByAgent[id] = cost
			}
		}
	}

	// Aggregate by connector
	rows, _ = p.pool.Query(ctx, `SELECT connector, SUM(cost_usd) FROM budget_ledger GROUP BY connector`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var cost float64
			if err := rows.Scan(&id, &cost); err == nil {
				report.ByConnector[id] = cost
			}
		}
	}

	// Aggregate by model
	rows, _ = p.pool.Query(ctx, `SELECT COALESCE(model, ''), SUM(cost_usd) FROM budget_ledger GROUP BY model`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var cost float64
			if err := rows.Scan(&id, &cost); err == nil {
				report.ByModel[id] = cost
			}
		}
	}

	return report, nil
}

// InsertAgentToken inserts an authorization token.
func (p *PostgresAppStore) InsertAgentToken(ctx context.Context, token *store.AgentToken) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO agent_tokens (id, agent_id, connectors, methods, budget_cap_usd, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, token.ID, token.AgentID, token.Connectors, token.Methods, token.BudgetCapUSD, toTime(token.ExpiresAt), toTime(token.CreatedAt))
	return err
}

// IsTokenRevoked checks if a token has been revoked.
func (p *PostgresAppStore) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	var count int
	err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM revoked_tokens WHERE token_id = $1`, tokenID).Scan(&count)
	return count > 0, err
}

// InsertRevokedToken marks a token as revoked.
func (p *PostgresAppStore) InsertRevokedToken(ctx context.Context, tokenID string) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO revoked_tokens (token_id, revoked_at) VALUES ($1, $2)
	`, tokenID, time.Now())
	return err
}

// GetCredential retrieves a credential by workspace and connector.
func (p *PostgresAppStore) GetCredential(ctx context.Context, workspaceID, connector string) (*store.Credential, error) {
	var c store.Credential
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, connector, label, key_hash, encrypted, last_used_at, created_at
		FROM credentials WHERE workspace_id = $1 AND connector = $2 ORDER BY created_at DESC LIMIT 1
	`, workspaceID, connector).Scan(&c.ID, &c.WorkspaceID, &c.Connector, &c.Label, &c.KeyHash, &c.Encrypted, &c.LastUsedAt, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// StoreCredential stores a credential (creates or updates).
func (p *PostgresAppStore) StoreCredential(ctx context.Context, c *store.Credential) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO credentials (id, workspace_id, connector, label, key_hash, encrypted, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (workspace_id, connector, key_hash) DO UPDATE SET
			label = excluded.label,
			encrypted = excluded.encrypted
	`, c.ID, c.WorkspaceID, c.Connector, c.Label, c.KeyHash, c.Encrypted, time.Now())
	return err
}

// DeleteCredential deletes a credential by ID.
func (p *PostgresAppStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM credentials WHERE id = $1`, id)
	return err
}

// WithTx runs a function within a transaction.
// Note: Transaction store is minimal and only supports create/insert operations.
func (p *PostgresAppStore) WithTx(ctx context.Context, fn func(store.AppStore) error) error {
	// For now, transactions are not fully implemented - just delegate to the non-transactional store
	// TODO: Implement proper transaction support with a transactional adapter
	return fn(p)
}

// Helper to convert UnixMilli int64 to time.Time
func toTime(ms int64) time.Time {
	return time.UnixMilli(ms)
}

// Helper to convert *int64 to *time.Time
func ptrToTime(ms *int64) *time.Time {
	if ms == nil {
		return nil
	}
	t := time.UnixMilli(*ms)
	return &t
}
