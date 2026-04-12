package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	postgresdb "github.com/kave-io/kave/server/internal/db/postgres"
)

// PostgresAppStore implements store.AppStore using Postgres via pgxpool.
type PostgresAppStore struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *PostgresAppStore {
	return &PostgresAppStore{pool: pool}
}

func (p *PostgresAppStore) Close() error {
	p.pool.Close()
	return nil
}

func (p *PostgresAppStore) Migrate(ctx context.Context) error {
	return postgresdb.Migrate(ctx, p.pool)
}

// ── OrgStore stubs ────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateOrg(_ context.Context, _ *control.Organization) error { return nil }
func (p *PostgresAppStore) GetOrg(_ context.Context, _ string) (*control.Organization, error) {
	return nil, nil
}
func (p *PostgresAppStore) GetOrgBySlug(_ context.Context, _ string) (*control.Organization, error) {
	return nil, nil
}

// ── UserStore stubs ───────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateUser(_ context.Context, _ *control.User) error { return nil }
func (p *PostgresAppStore) GetUser(_ context.Context, _ string) (*control.User, error) {
	return nil, nil
}
func (p *PostgresAppStore) GetUserByEmail(_ context.Context, _, _ string) (*control.User, error) {
	return nil, nil
}
func (p *PostgresAppStore) UpdateUser(_ context.Context, _ string, _ *control.UserUpdate) error {
	return nil
}

// ── MembershipStore stubs ─────────────────────────────────────────────────────

func (p *PostgresAppStore) AddMember(_ context.Context, _ *control.Membership) error { return nil }
func (p *PostgresAppStore) GetMembership(_ context.Context, _, _ string) (*control.Membership, error) {
	return nil, nil
}
func (p *PostgresAppStore) ListMembers(_ context.Context, _ string) ([]*control.Membership, error) {
	return nil, nil
}
func (p *PostgresAppStore) RemoveMember(_ context.Context, _, _ string) error { return nil }

// ── ProjectStore — maps to workspaces table ───────────────────────────────────

func (p *PostgresAppStore) CreateProject(ctx context.Context, proj *control.Project) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO workspaces (id, name, slug, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, proj.ID, proj.Name, proj.Slug, proj.Description, toTime(proj.CreatedAt), toTime(proj.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetProject(ctx context.Context, id string) (*control.Project, error) {
	var proj control.Project
	var createdAt, updatedAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, name, slug, description, created_at, updated_at FROM workspaces WHERE id = $1
	`, id).Scan(&proj.ID, &proj.Name, &proj.Slug, &proj.Description, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	proj.CreatedAt = createdAt.UnixMilli()
	proj.UpdatedAt = updatedAt.UnixMilli()
	return &proj, nil
}

func (p *PostgresAppStore) ListProjects(_ context.Context, _ string) ([]*control.Project, error) {
	return nil, nil
}

// ── EnvironmentStore stubs ────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateEnvironment(_ context.Context, _ *control.Environment) error {
	return nil
}
func (p *PostgresAppStore) GetEnvironment(_ context.Context, id string) (*control.Environment, error) {
	if id == "default" {
		return &control.Environment{ID: "default", ProjectID: "default", Name: "Default", Slug: "default"}, nil
	}
	return nil, nil
}
func (p *PostgresAppStore) GetEnvironmentBySlug(_ context.Context, projectID, slug string) (*control.Environment, error) {
	if slug == "default" {
		return &control.Environment{ID: "default", ProjectID: projectID, Name: "Default", Slug: "default"}, nil
	}
	return nil, nil
}
func (p *PostgresAppStore) ListEnvironments(_ context.Context, _ string) ([]*control.Environment, error) {
	return []*control.Environment{{ID: "default", ProjectID: "default", Name: "Default", Slug: "default"}}, nil
}

// ── AgentStore ────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateAgent(ctx context.Context, a *control.Agent) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	var budgetDollars *float64
	if a.MonthlyBudget != nil {
		d := a.MonthlyBudget.Dollars()
		budgetDollars = &d
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO agents (id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, a.ID, a.ProjectID, a.Name, a.Description, a.PolicyID, budgetDollars, metaJSON, toTime(a.CreatedAt), toTime(a.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetAgentByID(ctx context.Context, id string) (*control.Agent, error) {
	return p.getAgent(ctx, `SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at FROM agents WHERE id = $1`, id)
}

func (p *PostgresAppStore) GetAgentByName(ctx context.Context, envID, name string) (*control.Agent, error) {
	return p.getAgent(ctx, `SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at FROM agents WHERE workspace_id = $1 AND name = $2`, envID, name)
}

func (p *PostgresAppStore) getAgent(ctx context.Context, query string, args ...any) (*control.Agent, error) {
	var a control.Agent
	var metaJSON []byte
	var createdAt, updatedAt time.Time
	var budgetDollars *float64
	err := p.pool.QueryRow(ctx, query, args...).Scan(
		&a.ID, &a.ProjectID, &a.Name, &a.Description, &a.PolicyID, &budgetDollars, &metaJSON, &createdAt, &updatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if budgetDollars != nil {
		amt := money.FromDollars(*budgetDollars)
		a.MonthlyBudget = &amt
	}
	if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
		a.Metadata = make(map[string]any)
	}
	a.CreatedAt = createdAt.UnixMilli()
	a.UpdatedAt = updatedAt.UnixMilli()
	a.EnvID = "default"
	a.Status = control.AgentStatusActive
	return &a, nil
}

func (p *PostgresAppStore) UpdateAgent(ctx context.Context, id string, update *control.AgentUpdate) error {
	now := time.Now()
	metaJSON, _ := json.Marshal(update.Metadata)
	var budgetDollars *float64
	if update.MonthlyBudget != nil {
		d := update.MonthlyBudget.Dollars()
		budgetDollars = &d
	}
	_, err := p.pool.Exec(ctx, `
		UPDATE agents
		SET description = COALESCE($1, description),
		    policy_id = COALESCE($2, policy_id),
		    monthly_budget = COALESCE($3, monthly_budget),
		    metadata = COALESCE($4, metadata),
		    updated_at = $5
		WHERE id = $6
	`, update.Description, update.PolicyID, budgetDollars, metaJSON, now, id)
	return err
}

func (p *PostgresAppStore) ListAgents(ctx context.Context, envID string) ([]*control.Agent, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at
		FROM agents WHERE workspace_id = $1 ORDER BY name
	`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*control.Agent
	for rows.Next() {
		var a control.Agent
		var metaJSON []byte
		var createdAt, updatedAt time.Time
		var budgetDollars *float64
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Description, &a.PolicyID, &budgetDollars, &metaJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if budgetDollars != nil {
			amt := money.FromDollars(*budgetDollars)
			a.MonthlyBudget = &amt
		}
		if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
			a.Metadata = make(map[string]any)
		}
		a.CreatedAt = createdAt.UnixMilli()
		a.UpdatedAt = updatedAt.UnixMilli()
		a.EnvID = "default"
		a.Status = control.AgentStatusActive
		agents = append(agents, &a)
	}
	return agents, rows.Err()
}

// ── PolicyStore ───────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreatePolicy(ctx context.Context, pol *control.PolicyRecord) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO policies (id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, pol.ID, pol.ProjectID, pol.Name, pol.Description, pol.AllowedConnectors, pol.AllowedMethods, pol.BudgetCap.Dollars(), pol.Config, toTime(pol.CreatedAt), toTime(pol.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetPolicy(ctx context.Context, id string) (*control.PolicyRecord, error) {
	var pol control.PolicyRecord
	var createdAt, updatedAt time.Time
	var budgetDollars float64
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at
		FROM policies WHERE id = $1
	`, id).Scan(&pol.ID, &pol.ProjectID, &pol.Name, &pol.Description, &pol.AllowedConnectors, &pol.AllowedMethods, &budgetDollars, &pol.Config, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pol.BudgetCap = money.FromDollars(budgetDollars)
	pol.CreatedAt = createdAt.UnixMilli()
	pol.UpdatedAt = updatedAt.UnixMilli()
	pol.EnvID = "default"
	pol.Mode = control.PolicyModeEnforce
	pol.Status = control.PolicyStatusActive
	return &pol, nil
}

func (p *PostgresAppStore) GetAgentPolicy(ctx context.Context, agentID string) (*control.PolicyRecord, error) {
	var policyID *string
	err := p.pool.QueryRow(ctx, `SELECT policy_id FROM agents WHERE id = $1`, agentID).Scan(&policyID)
	if err != nil || policyID == nil {
		return nil, err
	}
	return p.GetPolicy(ctx, *policyID)
}

func (p *PostgresAppStore) ListPolicies(ctx context.Context, envID string) ([]*control.PolicyRecord, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at
		FROM policies WHERE workspace_id = $1 ORDER BY name
	`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*control.PolicyRecord
	for rows.Next() {
		var pol control.PolicyRecord
		var createdAt, updatedAt time.Time
		var budgetDollars float64
		if err := rows.Scan(&pol.ID, &pol.ProjectID, &pol.Name, &pol.Description, &pol.AllowedConnectors, &pol.AllowedMethods, &budgetDollars, &pol.Config, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		pol.BudgetCap = money.FromDollars(budgetDollars)
		pol.CreatedAt = createdAt.UnixMilli()
		pol.UpdatedAt = updatedAt.UnixMilli()
		pol.EnvID = "default"
		pol.Mode = control.PolicyModeEnforce
		pol.Status = control.PolicyStatusActive
		policies = append(policies, &pol)
	}
	return policies, rows.Err()
}

// ── RunStore ──────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateRun(ctx context.Context, r *runtimemodel.RunRecord) error {
	metaJSON, _ := json.Marshal(r.Metadata)
	var budgetDollars *float64
	if r.BudgetCap != 0 {
		d := r.BudgetCap.Dollars()
		budgetDollars = &d
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO runs (id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, r.ID, r.ProjectID, r.AgentID, r.PolicyID, r.Name, r.Status, budgetDollars, r.Spent.Dollars(),
		metaJSON, r.ErrorMessage, toTime(r.StartedAt), ptrToTime(r.EndedAt), toTime(r.CreatedAt), toTime(r.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetRunByID(ctx context.Context, id string) (*runtimemodel.RunRecord, error) {
	var r runtimemodel.RunRecord
	var metaJSON []byte
	var startedAt, createdAt, updatedAt time.Time
	var endedAt *time.Time
	var budgetDollars *float64
	var spentDollars float64
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at
		FROM runs WHERE id = $1
	`, id).Scan(&r.ID, &r.ProjectID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &budgetDollars, &spentDollars,
		&metaJSON, &r.ErrorMessage, &startedAt, &endedAt, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if budgetDollars != nil {
		r.BudgetCap = money.FromDollars(*budgetDollars)
	}
	r.Spent = money.FromDollars(spentDollars)
	if err := json.Unmarshal(metaJSON, &r.Metadata); err != nil {
		r.Metadata = make(map[string]any)
	}
	r.StartedAt = startedAt.UnixMilli()
	if endedAt != nil {
		ms := endedAt.UnixMilli()
		r.EndedAt = &ms
	}
	r.CreatedAt = createdAt.UnixMilli()
	r.UpdatedAt = updatedAt.UnixMilli()
	r.EnvID = "default"
	return &r, nil
}

func (p *PostgresAppStore) GetRunByIdempotencyKey(_ context.Context, _, _ string) (*runtimemodel.RunRecord, error) {
	return nil, nil
}

func (p *PostgresAppStore) UpdateRun(ctx context.Context, id string, update *runtimemodel.RunUpdate) error {
	now := time.Now()
	metaJSON, _ := json.Marshal(update.Metadata)
	var spentDollars *float64
	if update.Spent != nil {
		d := update.Spent.Dollars()
		spentDollars = &d
	}
	_, err := p.pool.Exec(ctx, `
		UPDATE runs
		SET status = COALESCE($1, status),
		    spent_usd = COALESCE($2, spent_usd),
		    error_message = COALESCE($3, error_message),
		    ended_at = COALESCE($4, ended_at),
		    metadata = COALESCE($5, metadata),
		    updated_at = $6
		WHERE id = $7
	`, update.Status, spentDollars, update.ErrorMessage, ptrToTime(update.EndedAt), metaJSON, now, id)
	return err
}

func (p *PostgresAppStore) ListRuns(ctx context.Context, filter *runtimemodel.RunFilter) ([]*runtimemodel.RunRecord, error) {
	query := `SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at FROM runs WHERE 1=1`
	var args []any
	n := 1

	if filter.ProjectID != "" {
		query += fmt.Sprintf(` AND workspace_id = $%d`, n)
		args = append(args, filter.ProjectID)
		n++
	}
	if filter.AgentID != "" {
		query += fmt.Sprintf(` AND agent_id = $%d`, n)
		args = append(args, filter.AgentID)
		n++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(` AND status = $%d`, n)
		args = append(args, filter.Status)
		n++
	}
	if filter.FromMs != nil {
		query += fmt.Sprintf(` AND started_at >= $%d`, n)
		args = append(args, toTime(*filter.FromMs))
		n++
	}
	if filter.ToMs != nil {
		query += fmt.Sprintf(` AND started_at <= $%d`, n)
		args = append(args, toTime(*filter.ToMs))
		n++
	}
	query += ` ORDER BY started_at DESC`
	if filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT $%d`, n)
		args = append(args, filter.Limit)
	}

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*runtimemodel.RunRecord
	for rows.Next() {
		var r runtimemodel.RunRecord
		var metaJSON []byte
		var startedAt, createdAt, updatedAt time.Time
		var endedAt *time.Time
		var budgetDollars *float64
		var spentDollars float64
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &budgetDollars, &spentDollars,
			&metaJSON, &r.ErrorMessage, &startedAt, &endedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if budgetDollars != nil {
			r.BudgetCap = money.FromDollars(*budgetDollars)
		}
		r.Spent = money.FromDollars(spentDollars)
		if err := json.Unmarshal(metaJSON, &r.Metadata); err != nil {
			r.Metadata = make(map[string]any)
		}
		r.StartedAt = startedAt.UnixMilli()
		if endedAt != nil {
			ms := endedAt.UnixMilli()
			r.EndedAt = &ms
		}
		r.CreatedAt = createdAt.UnixMilli()
		r.UpdatedAt = updatedAt.UnixMilli()
		r.EnvID = "default"
		runs = append(runs, &r)
	}
	return runs, rows.Err()
}

// ── ActionStore ───────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateAction(ctx context.Context, a *runtimemodel.ActionRecord) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	var inputStr *string
	if a.Input != nil {
		s := string(*a.Input)
		inputStr = &s
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO actions (id, run_id, action_type, connector, method, input, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, a.ID, a.RunID, a.ActionType, a.Connector, a.Method, inputStr, metaJSON, toTime(a.CreatedAt))
	return err
}

func (p *PostgresAppStore) GetAction(ctx context.Context, id string) (*runtimemodel.ActionRecord, error) {
	var a runtimemodel.ActionRecord
	var metaJSON []byte
	var inputStr *string
	var createdAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, run_id, action_type, connector, method, input, metadata, created_at FROM actions WHERE id = $1
	`, id).Scan(&a.ID, &a.RunID, &a.ActionType, &a.Connector, &a.Method, &inputStr, &metaJSON, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if inputStr != nil {
		b := []byte(*inputStr)
		a.Input = &b
	}
	if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
		a.Metadata = make(map[string]any)
	}
	a.CreatedAt = createdAt.UnixMilli()
	a.Source = runtimemodel.ActionSourceIntercepted
	a.Status = runtimemodel.ActionStatusCompleted
	return &a, nil
}

func (p *PostgresAppStore) ListActionsByRun(ctx context.Context, runID string) ([]*runtimemodel.ActionRecord, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, run_id, action_type, connector, method, input, metadata, created_at FROM actions WHERE run_id = $1 ORDER BY created_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []*runtimemodel.ActionRecord
	for rows.Next() {
		var a runtimemodel.ActionRecord
		var metaJSON []byte
		var inputStr *string
		var createdAt time.Time
		if err := rows.Scan(&a.ID, &a.RunID, &a.ActionType, &a.Connector, &a.Method, &inputStr, &metaJSON, &createdAt); err != nil {
			return nil, err
		}
		if inputStr != nil {
			b := []byte(*inputStr)
			a.Input = &b
		}
		if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
			a.Metadata = make(map[string]any)
		}
		a.CreatedAt = createdAt.UnixMilli()
		a.Source = runtimemodel.ActionSourceIntercepted
		a.Status = runtimemodel.ActionStatusCompleted
		actions = append(actions, &a)
	}
	return actions, rows.Err()
}

// ── CostStore ─────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) GetPriceBook(ctx context.Context) (*runtimemodel.PriceBook, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT version, provider, match, source, input_per_million, output_per_million, cache_read_per_million, cache_write_per_million
		FROM price_book_entries ORDER BY sort_order ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	book := &runtimemodel.PriceBook{Entries: []runtimemodel.PriceModel{}}
	for rows.Next() {
		var version string
		var entry runtimemodel.PriceModel
		if err := rows.Scan(&version, &entry.Provider, &entry.Match, &entry.Source, &entry.InputPerMillion, &entry.OutputPerMillion, &entry.CacheReadPerMillion, &entry.CacheWritePerMillion); err != nil {
			return nil, err
		}
		if book.Version == "" {
			book.Version = version
		}
		book.Entries = append(book.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(book.Entries) == 0 {
		return nil, nil
	}
	return book, nil
}

func (p *PostgresAppStore) SavePriceBook(ctx context.Context, book *runtimemodel.PriceBook) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM price_book_entries`); err != nil {
		return err
	}
	for i, entry := range book.Entries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO price_book_entries (id, version, provider, match, source, input_per_million, output_per_million, cache_read_per_million, cache_write_per_million, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, uuid.NewString(), book.Version, entry.Provider, entry.Match, entry.Source,
			entry.InputPerMillion, entry.OutputPerMillion, entry.CacheReadPerMillion, entry.CacheWritePerMillion, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *PostgresAppStore) InsertBudgetEntry(ctx context.Context, entry *runtimemodel.BudgetEntry) error {
	metaJSON, _ := json.Marshal(entry.Metadata)
	snapshotJSON, _ := json.Marshal(entry.PriceSnapshot)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO budget_ledger (workspace_id, agent_id, run_id, action_id, span_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, price_version, price_snapshot, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, entry.ProjectID, entry.AgentID, nullIfEmpty(entry.RunID), entry.ActionID, entry.SpanID,
		entry.Connector, entry.Model, entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens,
		entry.Cost.Dollars(), nullIfEmpty(entry.PriceVersion), snapshotJSON, metaJSON, toTime(entry.CreatedAt))
	return err
}

func (p *PostgresAppStore) AddRunSpend(ctx context.Context, runID string, cost money.Amount) error {
	_, err := p.pool.Exec(ctx, `UPDATE runs SET spent_usd = spent_usd + $1 WHERE id = $2`, cost.Dollars(), runID)
	return err
}

func (p *PostgresAppStore) SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (money.Amount, error) {
	var total float64
	err := p.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM budget_ledger WHERE agent_id = $1 AND created_at >= $2
	`, agentID, toTime(sinceMs)).Scan(&total)
	return money.FromDollars(total), err
}

func (p *PostgresAppStore) GetSpendReport(ctx context.Context, filter *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	where, args, _ := pgSpendWhere(filter, 1)

	var totalDollars float64
	var maxTime, minTime *time.Time
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(SUM(cost_usd), 0), MAX(created_at), MIN(created_at) FROM budget_ledger `+where, args...).Scan(&totalDollars, &maxTime, &minTime)
	if err != nil {
		return nil, err
	}

	report := &runtimemodel.SpendReport{
		Total:       money.FromDollars(totalDollars),
		ByProject:   make(map[string]money.Amount),
		ByEnv:       make(map[string]money.Amount),
		ByPolicy:    make(map[string]money.Amount),
		ByAgent:     make(map[string]money.Amount),
		ByConnector: make(map[string]money.Amount),
		ByModel:     make(map[string]money.Amount),
	}
	if minTime != nil {
		report.PeriodStart = minTime.UnixMilli()
	}
	if maxTime != nil {
		report.PeriodEnd = maxTime.UnixMilli()
	}

	p.pgAggregateDim(ctx, report.ByProject, "workspace_id", where, args)
	p.pgAggregateDim(ctx, report.ByAgent, "agent_id", where, args)
	p.pgAggregateDim(ctx, report.ByConnector, "connector", where, args)
	p.pgAggregateDim(ctx, report.ByModel, "COALESCE(model, '')", where, args)

	return report, nil
}

func (p *PostgresAppStore) pgAggregateDim(ctx context.Context, dest map[string]money.Amount, dim, where string, args []any) {
	rows, err := p.pool.Query(ctx, `SELECT `+dim+`, SUM(cost_usd) FROM budget_ledger `+where+` GROUP BY `+dim, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var cost float64
		if err := rows.Scan(&id, &cost); err == nil {
			dest[id] = money.FromDollars(cost)
		}
	}
}

// ── TokenStore ────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) InsertAgentToken(ctx context.Context, token *control.AgentToken) error {
	var budgetDollars *float64
	if token.BudgetCap != nil {
		d := token.BudgetCap.Dollars()
		budgetDollars = &d
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO agent_tokens (id, agent_id, connectors, methods, budget_cap_usd, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, token.ID, token.AgentID, token.Connectors, token.Methods, budgetDollars, toTime(token.ExpiresAt), toTime(token.CreatedAt))
	return err
}

func (p *PostgresAppStore) GetTokenByHash(_ context.Context, _ string) (*control.AgentToken, error) {
	return nil, nil
}
func (p *PostgresAppStore) RevokeToken(_ context.Context, _, _, _ string) error { return nil }
func (p *PostgresAppStore) TouchToken(_ context.Context, _ string) error         { return nil }

func (p *PostgresAppStore) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	var count int
	err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM revoked_tokens WHERE token_id = $1`, tokenID).Scan(&count)
	return count > 0, err
}

func (p *PostgresAppStore) InsertRevokedToken(ctx context.Context, tokenID string) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO revoked_tokens (token_id, revoked_at) VALUES ($1, $2)`, tokenID, time.Now())
	return err
}

// ── CredentialStore ───────────────────────────────────────────────────────────

func (p *PostgresAppStore) GetCredential(ctx context.Context, id string) (*control.ConnectorCredential, error) {
	var c control.ConnectorCredential
	var encrypted []byte
	var createdAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, connector, label, key_hash, encrypted, last_used_at, created_at
		FROM credentials WHERE id = $1
	`, id).Scan(&c.ID, &c.ProjectID, &c.ConnectorType, &c.Label, &c.KeyHash, &encrypted, &c.LastUsedAt, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.EncryptedBlob = encrypted
	c.CreatedAt = createdAt.UnixMilli()
	c.SourceType = control.CredSourceEncrypted
	c.Status = control.CredStatusActive
	return &c, nil
}

func (p *PostgresAppStore) StoreCredential(ctx context.Context, c *control.ConnectorCredential) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO credentials (id, workspace_id, connector, label, key_hash, encrypted, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (workspace_id, connector, key_hash) DO UPDATE SET
			label = excluded.label,
			encrypted = excluded.encrypted
	`, c.ID, c.ProjectID, c.ConnectorType, c.Label, c.KeyHash, c.EncryptedBlob, time.Now())
	return err
}

func (p *PostgresAppStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM credentials WHERE id = $1`, id)
	return err
}

func (p *PostgresAppStore) ListCredentials(_ context.Context, _ string) ([]*control.ConnectorCredential, error) {
	return nil, nil
}

func (p *PostgresAppStore) ResolveCredential(ctx context.Context, filter *control.CredentialFilter) (*control.ConnectorCredential, error) {
	var c control.ConnectorCredential
	var encrypted []byte
	var createdAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, connector, label, key_hash, encrypted, last_used_at, created_at
		FROM credentials WHERE workspace_id = $1 AND connector = $2 ORDER BY created_at DESC LIMIT 1
	`, filter.EnvID, filter.ConnectorType).Scan(&c.ID, &c.ProjectID, &c.ConnectorType, &c.Label, &c.KeyHash, &encrypted, &c.LastUsedAt, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.EncryptedBlob = encrypted
	c.CreatedAt = createdAt.UnixMilli()
	c.SourceType = control.CredSourceEncrypted
	c.Status = control.CredStatusActive
	return &c, nil
}

func (p *PostgresAppStore) RotateCredential(_ context.Context, _ string, _ []byte, _, _ string) error {
	return nil
}
func (p *PostgresAppStore) RevokeCredential(_ context.Context, _, _, _ string) error { return nil }
func (p *PostgresAppStore) TouchCredential(_ context.Context, _ string) error         { return nil }

// ── Transaction ───────────────────────────────────────────────────────────────

func (p *PostgresAppStore) WithTx(_ context.Context, fn func(store.AppStore) error) error {
	return fn(p)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func pgSpendWhere(f *runtimemodel.SpendFilter, startN int) (string, []any, int) {
	where := "WHERE 1=1"
	var args []any
	n := startN
	if f.ProjectID != "" {
		where += fmt.Sprintf(" AND workspace_id = $%d", n)
		args = append(args, f.ProjectID)
		n++
	}
	if f.AgentID != "" {
		where += fmt.Sprintf(" AND agent_id = $%d", n)
		args = append(args, f.AgentID)
		n++
	}
	if f.Connector != "" {
		where += fmt.Sprintf(" AND connector = $%d", n)
		args = append(args, f.Connector)
		n++
	}
	if f.Model != "" {
		where += fmt.Sprintf(" AND model = $%d", n)
		args = append(args, f.Model)
		n++
	}
	if f.FromMs != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", n)
		args = append(args, toTime(*f.FromMs))
		n++
	}
	if f.ToMs != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", n)
		args = append(args, toTime(*f.ToMs))
		n++
	}
	return where, args, n
}

func toTime(ms int64) time.Time       { return time.UnixMilli(ms) }
func ptrToTime(ms *int64) *time.Time {
	if ms == nil {
		return nil
	}
	t := time.UnixMilli(*ms)
	return &t
}
func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

var _ store.AppStore = (*PostgresAppStore)(nil)
