package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	dsn  string
}

func New(pool *pgxpool.Pool, dsn string) *PostgresAppStore {
	return &PostgresAppStore{pool: pool, dsn: dsn}
}

func (p *PostgresAppStore) Close() error {
	p.pool.Close()
	return nil
}

func (p *PostgresAppStore) Migrate(ctx context.Context) error {
	return postgresdb.Migrate(ctx, p.pool)
}

func (p *PostgresAppStore) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

func (p *PostgresAppStore) Stats(ctx context.Context) (map[string]any, error) {
	stats := map[string]any{
		"backend": "postgres",
		"dsn":     redactDSN(p.dsn),
	}
	tables := []string{
		"workspaces", "users", "memberships", "agents", "policies",
		"runs", "actions", "price_book", "fx_rates", "fx_currencies",
		"agent_tokens", "connector_credentials", "budgets",
	}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var n int64
		if err := p.pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			continue
		}
		counts[table] = n
	}
	stats["tables"] = counts
	return stats, nil
}

func redactDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	parts := strings.Fields(dsn)
	for i, part := range parts {
		if strings.HasPrefix(strings.ToLower(part), "password=") {
			parts[i] = "password=***"
		}
	}
	return strings.Join(parts, " ")
}

// ── OrgStore ─────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateOrg(ctx context.Context, o *control.Organization) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO workspaces (id, name, slug, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, o.ID, o.Name, o.Slug, "", toTime(o.CreatedAt), toTime(o.UpdatedAt))
	return err
}
func (p *PostgresAppStore) GetOrg(ctx context.Context, id string) (*control.Organization, error) {
	var o control.Organization
	var createdAt, updatedAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, name, slug, created_at, updated_at FROM workspaces WHERE id = $1
	`, id).Scan(&o.ID, &o.Name, &o.Slug, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o.CreatedAt = createdAt.UnixMilli()
	o.UpdatedAt = updatedAt.UnixMilli()
	return &o, nil
}
func (p *PostgresAppStore) GetOrgBySlug(ctx context.Context, slug string) (*control.Organization, error) {
	var o control.Organization
	var createdAt, updatedAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, name, slug, created_at, updated_at FROM workspaces WHERE slug = $1
	`, slug).Scan(&o.ID, &o.Name, &o.Slug, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o.CreatedAt = createdAt.UnixMilli()
	o.UpdatedAt = updatedAt.UnixMilli()
	return &o, nil
}
func (p *PostgresAppStore) ListOrgs(ctx context.Context, page store.Page) (store.PageResult[*control.Organization], error) {
	rows, err := p.pool.Query(ctx, `SELECT id, name, slug, created_at, updated_at FROM workspaces ORDER BY created_at ASC`)
	if err != nil {
		return store.PageResult[*control.Organization]{}, err
	}
	defer rows.Close()

	var items []*control.Organization
	for rows.Next() {
		var o control.Organization
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &createdAt, &updatedAt); err != nil {
			return store.PageResult[*control.Organization]{}, err
		}
		o.CreatedAt = createdAt.UnixMilli()
		o.UpdatedAt = updatedAt.UnixMilli()
		items = append(items, &o)
	}
	return store.Paginate(items, page), rows.Err()
}

// ── UserStore stubs ───────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateUser(_ context.Context, _ *control.User) error { return nil }
func (p *PostgresAppStore) GetUser(_ context.Context, _ string) (*control.User, error) {
	return nil, nil
}
func (p *PostgresAppStore) GetUserByEmail(_ context.Context, _, _ string) (*control.User, error) {
	return nil, nil
}
func (p *PostgresAppStore) UpdateUser(_ context.Context, _ string, update *control.UserUpdate) error {
	_ = update
	return nil
}

// ── RBAC stubs ───────────────────────────────────────────────────────────────

func (p *PostgresAppStore) InsertRole(_ context.Context, _ *control.Role) error { return nil }
func (p *PostgresAppStore) GetRole(_ context.Context, _ string) (*control.Role, error) {
	return nil, nil
}
func (p *PostgresAppStore) ListRoles(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Role], error) {
	return store.PageResult[*control.Role]{}, nil
}
func (p *PostgresAppStore) UpdateRole(_ context.Context, _ string, _ *control.Role) error { return nil }
func (p *PostgresAppStore) DeleteRole(_ context.Context, _ string) error                  { return nil }
func (p *PostgresAppStore) InsertBinding(_ context.Context, _ *control.Binding) error     { return nil }
func (p *PostgresAppStore) GetBinding(_ context.Context, _ string) (*control.Binding, error) {
	return nil, nil
}
func (p *PostgresAppStore) ListBindings(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Binding], error) {
	return store.PageResult[*control.Binding]{}, nil
}
func (p *PostgresAppStore) DeleteBinding(_ context.Context, _ string) error { return nil }

// ── MembershipStore stubs ─────────────────────────────────────────────────────

func (p *PostgresAppStore) AddMember(_ context.Context, _ *control.Membership) error { return nil }
func (p *PostgresAppStore) GetMembership(_ context.Context, _, _ string) (*control.Membership, error) {
	return nil, nil
}
func (p *PostgresAppStore) ListMembers(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Membership], error) {
	return store.PageResult[*control.Membership]{}, nil
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

func (p *PostgresAppStore) ListProjects(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Project], error) {
	return store.PageResult[*control.Project]{}, nil
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
func (p *PostgresAppStore) ListEnvironments(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Environment], error) {
	return store.PageResult[*control.Environment]{Items: []*control.Environment{{ID: "default", ProjectID: "default", Name: "Default", Slug: "default"}}}, nil
}

// ── AgentStore ────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateAgent(ctx context.Context, a *control.Agent) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	budgetAmount := ptrAmountToDB(a.MonthlyBudget)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO agents (id, workspace_id, name, description, policy_id, monthly_budget_amount_nanos, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, a.ID, a.ProjectID, a.Name, a.Description, a.PolicyID, budgetAmount, metaJSON, toTime(a.CreatedAt), toTime(a.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetAgentByID(ctx context.Context, id string) (*control.Agent, error) {
	return p.getAgent(ctx, `SELECT id, workspace_id, name, description, policy_id, monthly_budget_amount_nanos, metadata, created_at, updated_at FROM agents WHERE id = $1`, id)
}

func (p *PostgresAppStore) GetAgentByName(ctx context.Context, envID, name string) (*control.Agent, error) {
	return p.getAgent(ctx, `SELECT id, workspace_id, name, description, policy_id, monthly_budget_amount_nanos, metadata, created_at, updated_at FROM agents WHERE workspace_id = $1 AND name = $2`, envID, name)
}

func (p *PostgresAppStore) getAgent(ctx context.Context, query string, args ...any) (*control.Agent, error) {
	var a control.Agent
	var metaJSON []byte
	var createdAt, updatedAt time.Time
	var budgetAmount *int64
	err := p.pool.QueryRow(ctx, query, args...).Scan(
		&a.ID, &a.ProjectID, &a.Name, &a.Description, &a.PolicyID, &budgetAmount, &metaJSON, &createdAt, &updatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.MonthlyBudget = ptrAmountFromDB(budgetAmount)
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
	budgetAmount := ptrAmountToDB(update.MonthlyBudget)
	_, err := p.pool.Exec(ctx, `
		UPDATE agents
		SET description = COALESCE($1, description),
		    policy_id = COALESCE($2, policy_id),
		    monthly_budget_amount_nanos = COALESCE($3, monthly_budget_amount_nanos),
		    metadata = COALESCE($4, metadata),
		    updated_at = $5
		WHERE id = $6
	`, update.Description, update.PolicyID, budgetAmount, metaJSON, now, id)
	return err
}

func (p *PostgresAppStore) DeleteAgent(ctx context.Context, id, deletedBy string) error {
	now := time.Now()
	_, err := p.pool.Exec(ctx, `UPDATE agents SET deleted_at = $1, updated_at = $1 WHERE id = $2`, now, id)
	return err
}

func (p *PostgresAppStore) RestoreAgent(ctx context.Context, id, restoredBy string) error {
	now := time.Now()
	_, err := p.pool.Exec(ctx, `UPDATE agents SET deleted_at = NULL, updated_at = $1 WHERE id = $2`, now, id)
	return err
}

func (p *PostgresAppStore) ListAgents(ctx context.Context, envID string, page store.Page) (store.PageResult[*control.Agent], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, workspace_id, name, description, policy_id, monthly_budget_amount_nanos, metadata, created_at, updated_at
		FROM agents WHERE workspace_id = $1 ORDER BY name
	`, envID)
	if err != nil {
		return store.PageResult[*control.Agent]{}, err
	}
	defer rows.Close()

	var agents []*control.Agent
	for rows.Next() {
		var a control.Agent
		var metaJSON []byte
		var createdAt, updatedAt time.Time
		var budgetAmount *int64
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Description, &a.PolicyID, &budgetAmount, &metaJSON, &createdAt, &updatedAt); err != nil {
			return store.PageResult[*control.Agent]{}, err
		}
		a.MonthlyBudget = ptrAmountFromDB(budgetAmount)
		if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
			a.Metadata = make(map[string]any)
		}
		a.CreatedAt = createdAt.UnixMilli()
		a.UpdatedAt = updatedAt.UnixMilli()
		a.EnvID = "default"
		a.Status = control.AgentStatusActive
		agents = append(agents, &a)
	}
	return store.Paginate(agents, page), rows.Err()
}

// ── PolicyStore ───────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreatePolicy(ctx context.Context, pol *control.PolicyRecord) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO policies (id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_amount_nanos, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, pol.ID, pol.ProjectID, pol.Name, pol.Description, pol.AllowedConnectors, pol.AllowedMethods, amountToDB(pol.BudgetCap), pol.Config, toTime(pol.CreatedAt), toTime(pol.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetPolicy(ctx context.Context, id string) (*control.PolicyRecord, error) {
	var pol control.PolicyRecord
	var createdAt, updatedAt time.Time
	var budgetAmount int64
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_amount_nanos, config, created_at, updated_at
		FROM policies WHERE id = $1
	`, id).Scan(&pol.ID, &pol.ProjectID, &pol.Name, &pol.Description, &pol.AllowedConnectors, &pol.AllowedMethods, &budgetAmount, &pol.Config, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pol.BudgetCap = amountFromDB(budgetAmount)
	pol.CreatedAt = createdAt.UnixMilli()
	pol.UpdatedAt = updatedAt.UnixMilli()
	pol.EnvID = "default"
	pol.Mode = "enforce"
	pol.Status = "active"
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

func (p *PostgresAppStore) ListPolicies(ctx context.Context, envID string, page store.Page) (store.PageResult[*control.PolicyRecord], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_amount_nanos, config, created_at, updated_at
		FROM policies WHERE workspace_id = $1 ORDER BY name
	`, envID)
	if err != nil {
		return store.PageResult[*control.PolicyRecord]{}, err
	}
	defer rows.Close()

	var policies []*control.PolicyRecord
	for rows.Next() {
		var pol control.PolicyRecord
		var createdAt, updatedAt time.Time
		var budgetAmount int64
		if err := rows.Scan(&pol.ID, &pol.ProjectID, &pol.Name, &pol.Description, &pol.AllowedConnectors, &pol.AllowedMethods, &budgetAmount, &pol.Config, &createdAt, &updatedAt); err != nil {
			return store.PageResult[*control.PolicyRecord]{}, err
		}
		pol.BudgetCap = amountFromDB(budgetAmount)
		pol.CreatedAt = createdAt.UnixMilli()
		pol.UpdatedAt = updatedAt.UnixMilli()
		pol.EnvID = "default"
		pol.Mode = "enforce"
		pol.Status = "active"
		policies = append(policies, &pol)
	}
	return store.Paginate(policies, page), rows.Err()
}

// ── RunStore ──────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateRun(ctx context.Context, r *runtimemodel.RunRecord) error {
	metaJSON, _ := json.Marshal(r.Metadata)
	var budgetAmount *int64
	if r.BudgetCap != 0 {
		v := amountToDB(r.BudgetCap)
		budgetAmount = &v
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO runs (id, workspace_id, agent_id, policy_id, name, status, budget_cap_amount_nanos, spent_amount_nanos, metadata, error_message, started_at, ended_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, r.ID, r.ProjectID, r.AgentID, r.PolicyID, r.Name, r.Status, budgetAmount, amountToDB(r.Spent),
		metaJSON, r.ErrorMessage, toTime(r.StartedAt), ptrToTime(r.EndedAt), toTime(r.CreatedAt), toTime(r.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetRunByID(ctx context.Context, id string) (*runtimemodel.RunRecord, error) {
	var r runtimemodel.RunRecord
	var metaJSON []byte
	var startedAt, createdAt, updatedAt time.Time
	var endedAt *time.Time
	var budgetAmount *int64
	var spentAmount int64
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_amount_nanos, spent_amount_nanos, metadata, error_message, started_at, ended_at, created_at, updated_at
		FROM runs WHERE id = $1
	`, id).Scan(&r.ID, &r.ProjectID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &budgetAmount, &spentAmount,
		&metaJSON, &r.ErrorMessage, &startedAt, &endedAt, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if budgetAmount != nil {
		r.BudgetCap = amountFromDB(*budgetAmount)
	}
	r.Spent = amountFromDB(spentAmount)
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
	spentAmount := ptrAmountToDB(update.Spent)
	_, err := p.pool.Exec(ctx, `
		UPDATE runs
		SET status = COALESCE($1, status),
		    spent_amount_nanos = COALESCE($2, spent_amount_nanos),
		    error_message = COALESCE($3, error_message),
		    ended_at = COALESCE($4, ended_at),
		    metadata = COALESCE($5, metadata),
		    updated_at = $6
		WHERE id = $7
	`, update.Status, spentAmount, update.ErrorMessage, ptrToTime(update.EndedAt), metaJSON, now, id)
	return err
}

func (p *PostgresAppStore) ListRuns(ctx context.Context, filter *runtimemodel.RunFilter, page store.Page) (store.PageResult[*runtimemodel.RunRecord], error) {
	query := `SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_amount_nanos, spent_amount_nanos, metadata, error_message, started_at, ended_at, created_at, updated_at FROM runs WHERE 1=1`
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
	limit := page.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(` ORDER BY started_at DESC LIMIT $%d`, n)
	args = append(args, limit)

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return store.PageResult[*runtimemodel.RunRecord]{}, err
	}
	defer rows.Close()

	var runs []*runtimemodel.RunRecord
	for rows.Next() {
		var r runtimemodel.RunRecord
		var metaJSON []byte
		var startedAt, createdAt, updatedAt time.Time
		var endedAt *time.Time
		var budgetAmount *int64
		var spentAmount int64
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &budgetAmount, &spentAmount,
			&metaJSON, &r.ErrorMessage, &startedAt, &endedAt, &createdAt, &updatedAt); err != nil {
			return store.PageResult[*runtimemodel.RunRecord]{}, err
		}
		if budgetAmount != nil {
			r.BudgetCap = amountFromDB(*budgetAmount)
		}
		r.Spent = amountFromDB(spentAmount)
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
	return store.Paginate(runs, page), rows.Err()
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
	a.Source = "intercepted"
	a.Status = "completed"
	return &a, nil
}

func (p *PostgresAppStore) ListActionsByRun(ctx context.Context, runID string, page store.Page) (store.PageResult[*runtimemodel.ActionRecord], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, run_id, action_type, connector, method, input, metadata, created_at FROM actions WHERE run_id = $1 ORDER BY created_at ASC
	`, runID)
	if err != nil {
		return store.PageResult[*runtimemodel.ActionRecord]{}, err
	}
	defer rows.Close()

	var actions []*runtimemodel.ActionRecord
	for rows.Next() {
		var a runtimemodel.ActionRecord
		var metaJSON []byte
		var inputStr *string
		var createdAt time.Time
		if err := rows.Scan(&a.ID, &a.RunID, &a.ActionType, &a.Connector, &a.Method, &inputStr, &metaJSON, &createdAt); err != nil {
			return store.PageResult[*runtimemodel.ActionRecord]{}, err
		}
		if inputStr != nil {
			b := []byte(*inputStr)
			a.Input = &b
		}
		if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
			a.Metadata = make(map[string]any)
		}
		a.CreatedAt = createdAt.UnixMilli()
		a.Source = "intercepted"
		a.Status = "completed"
		actions = append(actions, &a)
	}
	return store.Paginate(actions, page), rows.Err()
}

// ── CostStore ─────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) GetPriceBook(ctx context.Context) (*runtimemodel.PriceBook, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT version, provider, match, source, currency,
		       input_per_million_amount_nanos, output_per_million_amount_nanos,
		       cache_read_per_million_amount_nanos, cache_write_per_million_amount_nanos,
		       reasoning_per_million_amount_nanos, audio_input_per_million_amount_nanos,
		       audio_output_per_million_amount_nanos, image_unit_price_amount_nanos,
		       per_request_amount_nanos, per_compute_ms_amount_nanos,
		       per_gb_stored_amount_nanos, per_gb_transferred_amount_nanos,
		       effective_from, effective_to, revision_note
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
		var effectiveTo *int64
		if err := rows.Scan(
			&version, &entry.Provider, &entry.Match, &entry.Source, &entry.Currency,
			&entry.InputPerMillion, &entry.OutputPerMillion,
			&entry.CacheReadPerMillion, &entry.CacheWritePerMillion,
			&entry.ReasoningPerMillion, &entry.AudioInputPerMillion,
			&entry.AudioOutputPerMillion, &entry.ImageUnitPrice,
			&entry.PerRequest, &entry.PerComputeMs,
			&entry.PerGBStored, &entry.PerGBTransferred,
			&entry.EffectiveFrom, &effectiveTo, &entry.RevisionNote,
		); err != nil {
			return nil, err
		}
		entry.EffectiveTo = effectiveTo
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
			INSERT INTO price_book_entries (
				id, version, provider, match, source, currency,
				input_per_million_amount_nanos, output_per_million_amount_nanos,
				cache_read_per_million_amount_nanos, cache_write_per_million_amount_nanos,
				reasoning_per_million_amount_nanos, audio_input_per_million_amount_nanos,
				audio_output_per_million_amount_nanos, image_unit_price_amount_nanos,
				per_request_amount_nanos, per_compute_ms_amount_nanos,
				per_gb_stored_amount_nanos, per_gb_transferred_amount_nanos,
				effective_from, effective_to, revision_note, sort_order
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		`, uuid.NewString(), book.Version, entry.Provider, entry.Match, entry.Source, entry.Currency,
			amountToDB(entry.InputPerMillion), amountToDB(entry.OutputPerMillion),
			amountToDB(entry.CacheReadPerMillion), amountToDB(entry.CacheWritePerMillion),
			amountToDB(entry.ReasoningPerMillion), amountToDB(entry.AudioInputPerMillion),
			amountToDB(entry.AudioOutputPerMillion), amountToDB(entry.ImageUnitPrice),
			amountToDB(entry.PerRequest), amountToDB(entry.PerComputeMs),
			amountToDB(entry.PerGBStored), amountToDB(entry.PerGBTransferred),
			entry.EffectiveFrom, entry.EffectiveTo, entry.RevisionNote, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *PostgresAppStore) ListFXRates(ctx context.Context) ([]runtimemodel.FXRateRecord, error) {
	rows, err := p.pool.Query(ctx, `SELECT base_currency, quote_currency, rate, provider, as_of_date::text, fetched_at FROM fx_rates ORDER BY base_currency, quote_currency`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []runtimemodel.FXRateRecord
	for rows.Next() {
		var item runtimemodel.FXRateRecord
		var fetchedAt time.Time
		if err := rows.Scan(&item.BaseCurrency, &item.QuoteCurrency, &item.Rate, &item.Provider, &item.AsOfDate, &fetchedAt); err != nil {
			return nil, err
		}
		item.FetchedAt = fetchedAt.UnixMilli()
		out = append(out, item)
	}
	return out, rows.Err()
}

func (p *PostgresAppStore) GetFXRate(ctx context.Context, base, quote money.CurrencyCode) (*runtimemodel.FXRateRecord, error) {
	var item runtimemodel.FXRateRecord
	var fetchedAt time.Time
	err := p.pool.QueryRow(ctx, `SELECT base_currency, quote_currency, rate, provider, as_of_date::text, fetched_at FROM fx_rates WHERE base_currency = $1 AND quote_currency = $2`, string(base), string(quote)).
		Scan(&item.BaseCurrency, &item.QuoteCurrency, &item.Rate, &item.Provider, &item.AsOfDate, &fetchedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.FetchedAt = fetchedAt.UnixMilli()
	return &item, nil
}

func (p *PostgresAppStore) UpsertFXRates(ctx context.Context, rates []runtimemodel.FXRateRecord) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, rate := range rates {
		if _, err := tx.Exec(ctx, `
			INSERT INTO fx_rates (base_currency, quote_currency, rate, provider, as_of_date, fetched_at)
			VALUES ($1, $2, $3, $4, $5::date, $6)
			ON CONFLICT (base_currency, quote_currency) DO UPDATE SET
				rate = EXCLUDED.rate,
				provider = EXCLUDED.provider,
				as_of_date = EXCLUDED.as_of_date,
				fetched_at = EXCLUDED.fetched_at
		`, string(rate.BaseCurrency), string(rate.QuoteCurrency), rate.Rate, rate.Provider, rate.AsOfDate, toTime(rate.FetchedAt)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *PostgresAppStore) ListFXCurrencies(ctx context.Context) ([]runtimemodel.FXCurrencyRecord, error) {
	rows, err := p.pool.Query(ctx, `SELECT code, name, symbol, fetched_at FROM fx_currencies ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []runtimemodel.FXCurrencyRecord
	for rows.Next() {
		var item runtimemodel.FXCurrencyRecord
		var fetchedAt time.Time
		if err := rows.Scan(&item.Code, &item.Name, &item.Symbol, &fetchedAt); err != nil {
			return nil, err
		}
		item.FetchedAt = fetchedAt.UnixMilli()
		out = append(out, item)
	}
	return out, rows.Err()
}

func (p *PostgresAppStore) UpsertFXCurrencies(ctx context.Context, currencies []runtimemodel.FXCurrencyRecord) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, item := range currencies {
		if _, err := tx.Exec(ctx, `
			INSERT INTO fx_currencies (code, name, symbol, fetched_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (code) DO UPDATE SET
				name = EXCLUDED.name,
				symbol = EXCLUDED.symbol,
				fetched_at = EXCLUDED.fetched_at
		`, string(item.Code), item.Name, item.Symbol, toTime(item.FetchedAt)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *PostgresAppStore) InsertBudgetEntry(ctx context.Context, entry *runtimemodel.BudgetEntry) error {
	metaJSON, _ := json.Marshal(entry.Metadata)
	snapshotJSON, _ := json.Marshal(entry.PriceSnapshot)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO budget_ledger (workspace_id, agent_id, run_id, action_id, span_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_amount_nanos, price_version, price_snapshot, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, entry.ProjectID, entry.AgentID, nullIfEmpty(entry.RunID), entry.ActionID, entry.SpanID,
		entry.Connector, entry.Model, entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens,
		amountToDB(entry.Cost), nullIfEmpty(entry.PriceVersion), snapshotJSON, metaJSON, toTime(entry.CreatedAt))
	return err
}

func (p *PostgresAppStore) AddRunSpend(ctx context.Context, runID string, cost money.Amount) error {
	_, err := p.pool.Exec(ctx, `UPDATE runs SET spent_amount_nanos = spent_amount_nanos + $1 WHERE id = $2`, amountToDB(cost), runID)
	return err
}

func (p *PostgresAppStore) SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (money.Amount, error) {
	var total int64
	err := p.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_amount_nanos), 0) FROM budget_ledger WHERE agent_id = $1 AND created_at >= $2
	`, agentID, toTime(sinceMs)).Scan(&total)
	return amountFromDB(total), err
}

func (p *PostgresAppStore) GetSpendReport(ctx context.Context, filter *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	where, args, _ := pgSpendWhere(filter, 1)

	var totalAmount int64
	var maxTime, minTime *time.Time
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(SUM(cost_amount_nanos), 0), MAX(created_at), MIN(created_at) FROM budget_ledger `+where, args...).Scan(&totalAmount, &maxTime, &minTime)
	if err != nil {
		return nil, err
	}

	report := &runtimemodel.SpendReport{
		Total:       amountFromDB(totalAmount),
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
	rows, err := p.pool.Query(ctx, `SELECT `+dim+`, SUM(cost_amount_nanos) FROM budget_ledger `+where+` GROUP BY `+dim, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var cost int64
		if err := rows.Scan(&id, &cost); err == nil {
			dest[id] = amountFromDB(cost)
		}
	}
}

// ── TokenStore ────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) InsertSession(_ context.Context, _ *control.Session) error { return nil }
func (p *PostgresAppStore) GetSessionByHash(_ context.Context, _ string) (*control.Session, error) {
	return nil, nil
}
func (p *PostgresAppStore) GetSession(_ context.Context, _ string) (*control.Session, error) {
	return nil, nil
}
func (p *PostgresAppStore) ListSessions(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Session], error) {
	return store.PageResult[*control.Session]{}, nil
}
func (p *PostgresAppStore) RevokeSession(_ context.Context, _, _ string) error { return nil }
func (p *PostgresAppStore) TouchSession(_ context.Context, _ string) error     { return nil }

func (p *PostgresAppStore) InsertAPIToken(_ context.Context, _ *control.APIToken) error { return nil }
func (p *PostgresAppStore) GetAPITokenByHash(_ context.Context, _ string) (*control.APIToken, error) {
	return nil, nil
}
func (p *PostgresAppStore) GetAPIToken(_ context.Context, _ string) (*control.APIToken, error) {
	return nil, nil
}
func (p *PostgresAppStore) ListAPITokens(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.APIToken], error) {
	return store.PageResult[*control.APIToken]{}, nil
}
func (p *PostgresAppStore) RevokeAPIToken(_ context.Context, _, _, _ string) error { return nil }
func (p *PostgresAppStore) TouchAPIToken(_ context.Context, _ string) error        { return nil }

func (p *PostgresAppStore) InsertAgentToken(_ context.Context, _ *control.AgentToken) error {
	return nil
}
func (p *PostgresAppStore) GetAgentTokenByHash(_ context.Context, _ string) (*control.AgentToken, error) {
	return nil, nil
}
func (p *PostgresAppStore) GetAgentToken(_ context.Context, _ string) (*control.AgentToken, error) {
	return nil, nil
}
func (p *PostgresAppStore) ListAgentTokens(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.AgentToken], error) {
	return store.PageResult[*control.AgentToken]{}, nil
}
func (p *PostgresAppStore) RevokeAgentToken(_ context.Context, _, _, _ string) error { return nil }
func (p *PostgresAppStore) TouchAgentToken(_ context.Context, _ string) error        { return nil }

func (p *PostgresAppStore) GetTokenByHash(ctx context.Context, hash string) (*control.AgentToken, error) {
	return p.GetAgentTokenByHash(ctx, hash)
}
func (p *PostgresAppStore) GetToken(ctx context.Context, id string) (*control.AgentToken, error) {
	return p.GetAgentToken(ctx, id)
}
func (p *PostgresAppStore) ListTokens(ctx context.Context, agentID string, page store.Page) (store.PageResult[*control.AgentToken], error) {
	return p.ListAgentTokens(ctx, agentID, page)
}
func (p *PostgresAppStore) RevokeToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	return p.RevokeAgentToken(ctx, tokenID, revokedBy, reason)
}
func (p *PostgresAppStore) TouchToken(ctx context.Context, tokenID string) error {
	return p.TouchAgentToken(ctx, tokenID)
}
func (p *PostgresAppStore) UpdatePolicy(_ context.Context, _ string, _ *control.PolicyUpdate) error {
	return nil
}
func (p *PostgresAppStore) DeletePolicy(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM policies WHERE id = $1`, id)
	return err
}
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

func (p *PostgresAppStore) ListCredentials(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.ConnectorCredential], error) {
	return store.PageResult[*control.ConnectorCredential]{}, nil
}

// ── BudgetStore ──────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateBudget(ctx context.Context, b *control.Budget) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO budgets (id, agent_id, hard_cap_nanos, soft_cap_nanos, period, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (agent_id) DO UPDATE SET
			hard_cap_nanos = EXCLUDED.hard_cap_nanos,
			soft_cap_nanos = EXCLUDED.soft_cap_nanos,
			period = EXCLUDED.period,
			updated_at = EXCLUDED.updated_at
	`, b.ID, b.AgentID, amountToDB(b.HardCap), amountToDB(b.SoftCap), b.Period, toTime(b.CreatedAt), toTime(b.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetBudget(ctx context.Context, agentID string) (*control.Budget, error) {
	var b control.Budget
	var createdAt, updatedAt time.Time
	var hardCap, softCap int64
	err := p.pool.QueryRow(ctx, `
		SELECT id, agent_id, hard_cap_nanos, soft_cap_nanos, period, created_at, updated_at
		FROM budgets WHERE agent_id = $1
	`, agentID).Scan(&b.ID, &b.AgentID, &hardCap, &softCap, &b.Period, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.HardCap = amountFromDB(hardCap)
	b.SoftCap = amountFromDB(softCap)
	b.CreatedAt = createdAt.UnixMilli()
	b.UpdatedAt = updatedAt.UnixMilli()
	return &b, nil
}

func (p *PostgresAppStore) DeleteBudget(ctx context.Context, agentID string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM budgets WHERE agent_id = $1`, agentID)
	return err
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
func (p *PostgresAppStore) TouchCredential(_ context.Context, _ string) error        { return nil }

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

func toTime(ms int64) time.Time { return time.UnixMilli(ms) }
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

func amountToDB(v money.Amount) int64 { return v.Nano() }

func ptrAmountToDB(v *money.Amount) *int64 {
	if v == nil {
		return nil
	}
	n := v.Nano()
	return &n
}

func amountFromDB(v int64) money.Amount { return money.Amount(v) }

func ptrAmountFromDB(v *int64) *money.Amount {
	if v == nil {
		return nil
	}
	a := amountFromDB(*v)
	return &a
}

var _ store.AppStore = (*PostgresAppStore)(nil)
