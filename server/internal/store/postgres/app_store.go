package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/ids"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
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
	return Migrate(ctx, p.pool)
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
		"agent_tokens", "credentials", "budgets",
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
		INSERT INTO workspaces (id, org_id, name, slug, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, o.ID, "", o.Name, o.Slug, "", toTime(o.CreatedAt), toTime(o.UpdatedAt))
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

// ── UserStore ─────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateUser(ctx context.Context, u *control.User) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO users (id, org_id, email, name, password_hash, status, last_login_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, u.ID, u.OrgID, u.Email, u.Name, u.PasswordHash, u.Status, ptrToTime(u.LastLoginAt), toTime(u.CreatedAt), toTime(u.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetUser(ctx context.Context, id string) (*control.User, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, email, name, password_hash, status, last_login_at, created_at, updated_at
		FROM users WHERE id = $1
	`, id)
	return scanPostgresUser(row)
}

func (p *PostgresAppStore) GetUserByEmail(ctx context.Context, orgID, email string) (*control.User, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, email, name, password_hash, status, last_login_at, created_at, updated_at
		FROM users WHERE org_id = $1 AND email = $2
	`, orgID, email)
	return scanPostgresUser(row)
}

func (p *PostgresAppStore) UpdateUser(ctx context.Context, id string, update *control.UserUpdate) error {
	if update == nil {
		return nil
	}
	var name any
	if update.Name != nil {
		name = *update.Name
	}
	var status any
	if update.Status != nil {
		status = *update.Status
	}
	var passwordHash any
	if update.PasswordHash != nil {
		passwordHash = *update.PasswordHash
	}
	_, err := p.pool.Exec(ctx, `
		UPDATE users SET
			name = COALESCE($1, name),
			status = COALESCE($2, status),
			password_hash = COALESCE($3, password_hash),
			last_login_at = COALESCE($4, last_login_at),
			updated_at = $5
		WHERE id = $6
	`, name, status, passwordHash, ptrToTime(update.LastLoginAt), time.Now(), id)
	return err
}

func scanPostgresUser(scanner postgresScanner) (*control.User, error) {
	var u control.User
	var lastLoginAt *time.Time
	var createdAt, updatedAt time.Time
	err := scanner.Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash, &u.Status, &lastLoginAt, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.LastLoginAt = timeToMillisPtr(lastLoginAt)
	u.CreatedAt = createdAt.UnixMilli()
	u.UpdatedAt = updatedAt.UnixMilli()
	return &u, nil
}

// ── RBAC ──────────────────────────────────────────────────────────────────────

func (p *PostgresAppStore) InsertRole(ctx context.Context, role *control.Role) error {
	permissions, _ := json.Marshal(role.Permissions)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO roles (id, org_id, name, permissions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, role.ID, role.OrgID, role.Name, string(permissions), toTime(role.CreatedAt), toTime(role.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetRole(ctx context.Context, id string) (*control.Role, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, name, permissions, created_at, updated_at
		FROM roles WHERE id = $1
	`, id)
	return scanPostgresRole(row)
}

func (p *PostgresAppStore) ListRoles(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Role], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, org_id, name, permissions, created_at, updated_at
		FROM roles WHERE org_id = $1 ORDER BY name ASC
	`, orgID)
	if err != nil {
		return store.PageResult[*control.Role]{}, err
	}
	defer rows.Close()

	var roles []*control.Role
	for rows.Next() {
		role, err := scanPostgresRole(rows)
		if err != nil {
			return store.PageResult[*control.Role]{}, err
		}
		roles = append(roles, role)
	}
	return store.Paginate(roles, page), rows.Err()
}

func (p *PostgresAppStore) UpdateRole(ctx context.Context, id string, role *control.Role) error {
	permissions, _ := json.Marshal(role.Permissions)
	_, err := p.pool.Exec(ctx, `
		UPDATE roles SET name = $1, permissions = $2, updated_at = $3 WHERE id = $4
	`, role.Name, string(permissions), toTime(role.UpdatedAt), id)
	return err
}

func (p *PostgresAppStore) DeleteRole(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	return err
}

func (p *PostgresAppStore) InsertBinding(ctx context.Context, binding *control.Binding) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO bindings (id, org_id, role_id, subject, scope, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, binding.ID, binding.OrgID, binding.RoleID, binding.Subject, binding.Scope, toTime(binding.CreatedAt))
	return err
}

func (p *PostgresAppStore) GetBinding(ctx context.Context, id string) (*control.Binding, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, role_id, subject, scope, created_at
		FROM bindings WHERE id = $1
	`, id)
	return scanPostgresBinding(row)
}

func (p *PostgresAppStore) ListBindings(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Binding], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, org_id, role_id, subject, scope, created_at
		FROM bindings WHERE org_id = $1 ORDER BY created_at ASC
	`, orgID)
	if err != nil {
		return store.PageResult[*control.Binding]{}, err
	}
	defer rows.Close()

	var bindings []*control.Binding
	for rows.Next() {
		binding, err := scanPostgresBinding(rows)
		if err != nil {
			return store.PageResult[*control.Binding]{}, err
		}
		bindings = append(bindings, binding)
	}
	return store.Paginate(bindings, page), rows.Err()
}

func (p *PostgresAppStore) DeleteBinding(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM bindings WHERE id = $1`, id)
	return err
}

func scanPostgresRole(scanner postgresScanner) (*control.Role, error) {
	var role control.Role
	var permissions string
	var createdAt, updatedAt time.Time
	err := scanner.Scan(&role.ID, &role.OrgID, &role.Name, &permissions, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(permissions), &role.Permissions); err != nil {
		role.Permissions = nil
	}
	role.CreatedAt = createdAt.UnixMilli()
	role.UpdatedAt = updatedAt.UnixMilli()
	return &role, nil
}

func scanPostgresBinding(scanner postgresScanner) (*control.Binding, error) {
	var binding control.Binding
	var createdAt time.Time
	err := scanner.Scan(&binding.ID, &binding.OrgID, &binding.RoleID, &binding.Subject, &binding.Scope, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	binding.CreatedAt = createdAt.UnixMilli()
	return &binding, nil
}

// ── MembershipStore ───────────────────────────────────────────────────────────

func (p *PostgresAppStore) AddMember(ctx context.Context, m *control.Membership) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO memberships (id, org_id, user_id, role, invited_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, m.ID, m.OrgID, m.UserID, m.Role, m.InvitedBy, toTime(m.CreatedAt))
	return err
}

func (p *PostgresAppStore) GetMembership(ctx context.Context, orgID, userID string) (*control.Membership, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, user_id, role, invited_by, created_at
		FROM memberships WHERE org_id = $1 AND user_id = $2
	`, orgID, userID)
	return scanPostgresMembership(row)
}

func (p *PostgresAppStore) ListMembers(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Membership], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, org_id, user_id, role, invited_by, created_at
		FROM memberships WHERE org_id = $1 ORDER BY created_at ASC
	`, orgID)
	if err != nil {
		return store.PageResult[*control.Membership]{}, err
	}
	defer rows.Close()

	var memberships []*control.Membership
	for rows.Next() {
		m, err := scanPostgresMembership(rows)
		if err != nil {
			return store.PageResult[*control.Membership]{}, err
		}
		memberships = append(memberships, m)
	}
	return store.Paginate(memberships, page), rows.Err()
}

func (p *PostgresAppStore) RemoveMember(ctx context.Context, orgID, userID string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM memberships WHERE org_id = $1 AND user_id = $2`, orgID, userID)
	return err
}

func scanPostgresMembership(scanner postgresScanner) (*control.Membership, error) {
	var m control.Membership
	var createdAt time.Time
	err := scanner.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.InvitedBy, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.CreatedAt = createdAt.UnixMilli()
	return &m, nil
}

// ── ProjectStore — maps to workspaces table ───────────────────────────────────

func (p *PostgresAppStore) CreateProject(ctx context.Context, proj *control.Project) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO workspaces (id, org_id, name, slug, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, proj.ID, proj.OrgID, proj.Name, proj.Slug, proj.Description, toTime(proj.CreatedAt), toTime(proj.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetProject(ctx context.Context, id string) (*control.Project, error) {
	var proj control.Project
	var createdAt, updatedAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, org_id, name, slug, description, created_at, updated_at FROM workspaces WHERE id = $1
	`, id).Scan(&proj.ID, &proj.OrgID, &proj.Name, &proj.Slug, &proj.Description, &createdAt, &updatedAt)
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

func (p *PostgresAppStore) ListProjects(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Project], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, org_id, name, slug, description, created_at, updated_at
		FROM workspaces WHERE org_id = $1 ORDER BY name ASC
	`, orgID)
	if err != nil {
		return store.PageResult[*control.Project]{}, err
	}
	defer rows.Close()

	var projects []*control.Project
	for rows.Next() {
		var proj control.Project
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&proj.ID, &proj.OrgID, &proj.Name, &proj.Slug, &proj.Description, &createdAt, &updatedAt); err != nil {
			return store.PageResult[*control.Project]{}, err
		}
		proj.CreatedAt = createdAt.UnixMilli()
		proj.UpdatedAt = updatedAt.UnixMilli()
		projects = append(projects, &proj)
	}
	return store.Paginate(projects, page), rows.Err()
}

// ── EnvironmentStore ──────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateEnvironment(ctx context.Context, e *control.Environment) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO environments (id, project_id, name, slug, type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, e.ID, e.ProjectID, e.Name, e.Slug, e.Type, toTime(e.CreatedAt), toTime(e.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetEnvironment(ctx context.Context, id string) (*control.Environment, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, project_id, name, slug, type, created_at, updated_at
		FROM environments WHERE id = $1
	`, id)
	return scanPostgresEnvironment(row)
}

func (p *PostgresAppStore) GetEnvironmentBySlug(ctx context.Context, projectID, slug string) (*control.Environment, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, project_id, name, slug, type, created_at, updated_at
		FROM environments WHERE project_id = $1 AND slug = $2
	`, projectID, slug)
	return scanPostgresEnvironment(row)
}

func (p *PostgresAppStore) ListEnvironments(ctx context.Context, projectID string, page store.Page) (store.PageResult[*control.Environment], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, project_id, name, slug, type, created_at, updated_at
		FROM environments WHERE project_id = $1 ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return store.PageResult[*control.Environment]{}, err
	}
	defer rows.Close()

	var environments []*control.Environment
	for rows.Next() {
		e, err := scanPostgresEnvironment(rows)
		if err != nil {
			return store.PageResult[*control.Environment]{}, err
		}
		environments = append(environments, e)
	}
	return store.Paginate(environments, page), rows.Err()
}

func scanPostgresEnvironment(scanner postgresScanner) (*control.Environment, error) {
	var e control.Environment
	var createdAt, updatedAt time.Time
	err := scanner.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Slug, &e.Type, &createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.CreatedAt = createdAt.UnixMilli()
	e.UpdatedAt = updatedAt.UnixMilli()
	return &e, nil
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
	var casbinDoc any
	if pol.CasbinDocument != "" {
		casbinDoc = pol.CasbinDocument
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO policies (id, workspace_id, name, description, allowed_connectors, allowed_methods, casbin_document, budget_cap_amount_nanos, config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, pol.ID, pol.ProjectID, pol.Name, pol.Description, pol.AllowedConnectors, pol.AllowedMethods, casbinDoc, amountToDB(pol.BudgetCap), pol.Config, toTime(pol.CreatedAt), toTime(pol.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetPolicy(ctx context.Context, id string) (*control.PolicyRecord, error) {
	var pol control.PolicyRecord
	var createdAt, updatedAt time.Time
	var budgetAmount int64
	err := p.pool.QueryRow(ctx, `
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, COALESCE(casbin_document, ''), budget_cap_amount_nanos, config, created_at, updated_at
		FROM policies WHERE id = $1
	`, id).Scan(&pol.ID, &pol.ProjectID, &pol.Name, &pol.Description, &pol.AllowedConnectors, &pol.AllowedMethods, &pol.CasbinDocument, &budgetAmount, &pol.Config, &createdAt, &updatedAt)
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
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, COALESCE(casbin_document, ''), budget_cap_amount_nanos, config, created_at, updated_at
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
		if err := rows.Scan(&pol.ID, &pol.ProjectID, &pol.Name, &pol.Description, &pol.AllowedConnectors, &pol.AllowedMethods, &pol.CasbinDocument, &budgetAmount, &pol.Config, &createdAt, &updatedAt); err != nil {
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
		INSERT INTO runs (
			id, project_id, env_id, agent_id, policy_id, name, status,
			budget_cap_amount_nanos, spent_amount_nanos, metadata, error_message,
			trigger_type, trigger_id, correlation_id, session_id, idempotency_key,
			started_at, ended_at, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`, r.ID, r.ProjectID, r.EnvID, r.AgentID, r.PolicyID, r.Name, r.Status, budgetAmount, amountToDB(r.Spent),
		metaJSON, r.ErrorMessage, r.TriggerType, r.TriggerID, r.CorrelationID, r.SessionID, r.IdempotencyKey,
		toTime(r.StartedAt), ptrToTime(r.EndedAt), toTime(r.CreatedAt), toTime(r.UpdatedAt))
	return err
}

func (p *PostgresAppStore) GetRunByID(ctx context.Context, id string) (*runtimemodel.RunRecord, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, project_id, env_id, agent_id, policy_id, name, status,
		       budget_cap_amount_nanos, spent_amount_nanos, metadata, error_message,
		       trigger_type, trigger_id, correlation_id, session_id, idempotency_key,
		       started_at, ended_at, created_at, updated_at
		FROM runs WHERE id = $1
	`, id)
	r, err := scanPostgresRun(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (p *PostgresAppStore) GetRunByIdempotencyKey(ctx context.Context, envID, key string) (*runtimemodel.RunRecord, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, project_id, env_id, agent_id, policy_id, name, status,
		       budget_cap_amount_nanos, spent_amount_nanos, metadata, error_message,
		       trigger_type, trigger_id, correlation_id, session_id, idempotency_key,
		       started_at, ended_at, created_at, updated_at
		FROM runs WHERE env_id = $1 AND idempotency_key = $2
	`, envID, key)
	r, err := scanPostgresRun(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return r, err
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
	query := `
		SELECT id, project_id, env_id, agent_id, policy_id, name, status,
		       budget_cap_amount_nanos, spent_amount_nanos, metadata, error_message,
		       trigger_type, trigger_id, correlation_id, session_id, idempotency_key,
		       started_at, ended_at, created_at, updated_at
		FROM runs WHERE 1=1`
	var args []any
	n := 1

	if filter.ProjectID != "" {
		query += fmt.Sprintf(` AND project_id = $%d`, n)
		args = append(args, filter.ProjectID)
		n++
	}
	if filter.EnvID != "" {
		query += fmt.Sprintf(` AND env_id = $%d`, n)
		args = append(args, filter.EnvID)
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
		r, err := scanPostgresRun(rows)
		if err != nil {
			return store.PageResult[*runtimemodel.RunRecord]{}, err
		}
		runs = append(runs, r)
	}
	return store.Paginate(runs, page), rows.Err()
}

type postgresScanner interface {
	Scan(dest ...any) error
}

func scanPostgresRun(scanner postgresScanner) (*runtimemodel.RunRecord, error) {
	var r runtimemodel.RunRecord
	var metaJSON []byte
	var startedAt, createdAt, updatedAt time.Time
	var endedAt *time.Time
	var budgetAmount *int64
	var spentAmount int64
	err := scanner.Scan(
		&r.ID, &r.ProjectID, &r.EnvID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status,
		&budgetAmount, &spentAmount, &metaJSON, &r.ErrorMessage,
		&r.TriggerType, &r.TriggerID, &r.CorrelationID, &r.SessionID, &r.IdempotencyKey,
		&startedAt, &endedAt, &createdAt, &updatedAt,
	)
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
	return &r, nil
}

// ── ActionStore ───────────────────────────────────────────────────────────────

func (p *PostgresAppStore) CreateAction(ctx context.Context, a *runtimemodel.ActionRecord) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO actions (
			id, run_id, agent_id, project_id, env_id, parent_id,
			action_type, connector, method, input, output, error,
			started_at, ended_at, depth, seq, status, source,
			metadata, attempt, max_attempts, retry_reason, provider_req_id, external_id, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23, $24, $25
		)
	`, a.ID, a.RunID, a.AgentID, a.ProjectID, a.EnvID, a.ParentID,
		a.ActionType, a.Connector, a.Method, jsonString(a.Input), jsonString(a.Output), a.Error,
		ptrToTime(a.StartedAt), ptrToTime(a.EndedAt), a.Depth, a.Seq, a.Status, a.Source,
		metaJSON, a.Attempt, a.MaxAttempts, a.RetryReason, a.ProviderReqID, a.ExternalID, toTime(a.CreatedAt))
	return err
}

func (p *PostgresAppStore) GetAction(ctx context.Context, id string) (*runtimemodel.ActionRecord, error) {
	var a runtimemodel.ActionRecord
	var metaJSON []byte
	var inputStr, outputStr *string
	var startedAt, endedAt *time.Time
	var createdAt time.Time
	err := p.pool.QueryRow(ctx, `
		SELECT id, run_id, agent_id, project_id, env_id, parent_id,
		       action_type, connector, method, input, output, error,
		       started_at, ended_at, depth, seq, status, source,
		       metadata, attempt, max_attempts, retry_reason, provider_req_id, external_id, created_at
		FROM actions WHERE id = $1
	`, id).Scan(
		&a.ID, &a.RunID, &a.AgentID, &a.ProjectID, &a.EnvID, &a.ParentID,
		&a.ActionType, &a.Connector, &a.Method, &inputStr, &outputStr, &a.Error,
		&startedAt, &endedAt, &a.Depth, &a.Seq, &a.Status, &a.Source,
		&metaJSON, &a.Attempt, &a.MaxAttempts, &a.RetryReason, &a.ProviderReqID, &a.ExternalID, &createdAt)
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
	if outputStr != nil {
		b := []byte(*outputStr)
		a.Output = &b
	}
	if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
		a.Metadata = make(map[string]any)
	}
	if startedAt != nil {
		ms := startedAt.UnixMilli()
		a.StartedAt = &ms
	}
	if endedAt != nil {
		ms := endedAt.UnixMilli()
		a.EndedAt = &ms
	}
	a.CreatedAt = createdAt.UnixMilli()
	return &a, nil
}

func (p *PostgresAppStore) UpdateAction(ctx context.Context, id string, update *runtimemodel.ActionUpdate) error {
	if update == nil {
		return nil
	}
	set := make([]string, 0, 8)
	args := make([]any, 0, 9)
	add := func(expr string, value any) {
		args = append(args, value)
		set = append(set, fmt.Sprintf("%s = $%d", expr, len(args)))
	}
	if update.Output != nil {
		add("output", jsonString(update.Output))
	}
	if update.Error != nil {
		add("error", *update.Error)
	}
	if update.StartedAt != nil {
		add("started_at", toTime(*update.StartedAt))
	}
	if update.EndedAt != nil {
		add("ended_at", toTime(*update.EndedAt))
	}
	if update.Status != nil {
		add("status", *update.Status)
	}
	if update.Metadata != nil {
		metaJSON, _ := json.Marshal(update.Metadata)
		add("metadata", metaJSON)
	}
	if update.ProviderReqID != nil {
		add("provider_req_id", *update.ProviderReqID)
	}
	if len(set) == 0 {
		return nil
	}
	args = append(args, id)
	_, err := p.pool.Exec(ctx, "UPDATE actions SET "+strings.Join(set, ", ")+" WHERE id = $"+fmt.Sprint(len(args)), args...)
	return err
}

func (p *PostgresAppStore) ListActionsByRun(ctx context.Context, runID string, page store.Page) (store.PageResult[*runtimemodel.ActionRecord], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, run_id, agent_id, project_id, env_id, parent_id,
		       action_type, connector, method, input, output, error,
		       started_at, ended_at, depth, seq, status, source,
		       metadata, attempt, max_attempts, retry_reason, provider_req_id, external_id, created_at
		FROM actions WHERE run_id = $1 ORDER BY created_at ASC
	`, runID)
	if err != nil {
		return store.PageResult[*runtimemodel.ActionRecord]{}, err
	}
	defer rows.Close()

	var actions []*runtimemodel.ActionRecord
	for rows.Next() {
		var a runtimemodel.ActionRecord
		var metaJSON []byte
		var inputStr, outputStr *string
		var startedAt, endedAt *time.Time
		var createdAt time.Time
		if err := rows.Scan(
			&a.ID, &a.RunID, &a.AgentID, &a.ProjectID, &a.EnvID, &a.ParentID,
			&a.ActionType, &a.Connector, &a.Method, &inputStr, &outputStr, &a.Error,
			&startedAt, &endedAt, &a.Depth, &a.Seq, &a.Status, &a.Source,
			&metaJSON, &a.Attempt, &a.MaxAttempts, &a.RetryReason, &a.ProviderReqID, &a.ExternalID, &createdAt); err != nil {
			return store.PageResult[*runtimemodel.ActionRecord]{}, err
		}
		if inputStr != nil {
			b := []byte(*inputStr)
			a.Input = &b
		}
		if outputStr != nil {
			b := []byte(*outputStr)
			a.Output = &b
		}
		if err := json.Unmarshal(metaJSON, &a.Metadata); err != nil {
			a.Metadata = make(map[string]any)
		}
		if startedAt != nil {
			ms := startedAt.UnixMilli()
			a.StartedAt = &ms
		}
		if endedAt != nil {
			ms := endedAt.UnixMilli()
			a.EndedAt = &ms
		}
		a.CreatedAt = createdAt.UnixMilli()
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
		`, ids.New("psn"), book.Version, entry.Provider, entry.Match, entry.Source, entry.Currency,
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
	snapshotJSON, _ := json.Marshal(entry.PriceSnapshot)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO budget_ledger (workspace_id, agent_id, run_id, action_id, span_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_amount_nanos, price_version, price_snapshot, blocked, block_reason, block_period, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`, entry.ProjectID, entry.AgentID, nullIfEmpty(entry.RunID), entry.ActionID, entry.SpanID,
		entry.Connector, entry.Model, entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens,
		amountToDB(entry.Cost), nullIfEmpty(entry.PriceVersion), snapshotJSON,
		entry.Blocked, nullIfEmpty(entry.BlockReason), nullIfEmpty(entry.BlockPeriod), toTime(entry.CreatedAt))
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

func (p *PostgresAppStore) InsertSession(ctx context.Context, session *control.Session) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO sessions (id, org_id, user_id, token_hash, expires_at, created_at, last_used_at, user_agent, ip, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, session.ID, session.OrgID, session.UserID, session.TokenHash, toTime(session.ExpiresAt),
		toTime(session.CreatedAt), ptrToTime(session.LastUsedAt), session.UserAgent, session.IP, ptrToTime(session.RevokedAt))
	return err
}

func (p *PostgresAppStore) GetSessionByHash(ctx context.Context, hash string) (*control.Session, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, user_id, token_hash, expires_at, created_at, last_used_at, user_agent, ip, revoked_at
		FROM sessions WHERE token_hash = $1
	`, []byte(hash))
	return scanPostgresSession(row)
}

func (p *PostgresAppStore) GetSession(ctx context.Context, id string) (*control.Session, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, user_id, token_hash, expires_at, created_at, last_used_at, user_agent, ip, revoked_at
		FROM sessions WHERE id = $1
	`, id)
	return scanPostgresSession(row)
}

func (p *PostgresAppStore) ListSessions(ctx context.Context, userID string, page store.Page) (store.PageResult[*control.Session], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, org_id, user_id, token_hash, expires_at, created_at, last_used_at, user_agent, ip, revoked_at
		FROM sessions WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return store.PageResult[*control.Session]{}, err
	}
	defer rows.Close()

	var items []*control.Session
	for rows.Next() {
		item, err := scanPostgresSession(rows)
		if err != nil {
			return store.PageResult[*control.Session]{}, err
		}
		items = append(items, item)
	}
	return store.Paginate(items, page), rows.Err()
}

func (p *PostgresAppStore) RevokeSession(ctx context.Context, sessionID, revokedBy string) error {
	_, err := p.pool.Exec(ctx, `UPDATE sessions SET revoked_at = $1 WHERE id = $2`, time.Now(), sessionID)
	return err
}

func (p *PostgresAppStore) TouchSession(ctx context.Context, sessionID string) error {
	_, err := p.pool.Exec(ctx, `UPDATE sessions SET last_used_at = $1 WHERE id = $2`, time.Now(), sessionID)
	return err
}

func (p *PostgresAppStore) InsertAPIToken(ctx context.Context, token *control.APIToken) error {
	scopes, _ := json.Marshal(token.Scopes)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO api_tokens (id, org_id, user_id, name, token_hash, scopes, expires_at, last_used_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, token.ID, token.OrgID, token.UserID, token.Name, token.TokenHash, string(scopes),
		ptrToTime(token.ExpiresAt), ptrToTime(token.LastUsedAt), ptrToTime(token.RevokedAt), toTime(token.CreatedAt))
	return err
}

func (p *PostgresAppStore) GetAPITokenByHash(ctx context.Context, hash string) (*control.APIToken, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, user_id, name, token_hash, scopes, expires_at, last_used_at, revoked_at, created_at
		FROM api_tokens WHERE token_hash = $1
	`, []byte(hash))
	return scanPostgresAPIToken(row)
}

func (p *PostgresAppStore) GetAPIToken(ctx context.Context, id string) (*control.APIToken, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, user_id, name, token_hash, scopes, expires_at, last_used_at, revoked_at, created_at
		FROM api_tokens WHERE id = $1
	`, id)
	return scanPostgresAPIToken(row)
}

func (p *PostgresAppStore) ListAPITokens(ctx context.Context, userID string, page store.Page) (store.PageResult[*control.APIToken], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, org_id, user_id, name, token_hash, scopes, expires_at, last_used_at, revoked_at, created_at
		FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return store.PageResult[*control.APIToken]{}, err
	}
	defer rows.Close()

	var items []*control.APIToken
	for rows.Next() {
		item, err := scanPostgresAPIToken(rows)
		if err != nil {
			return store.PageResult[*control.APIToken]{}, err
		}
		items = append(items, item)
	}
	return store.Paginate(items, page), rows.Err()
}

func (p *PostgresAppStore) RevokeAPIToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	_, err := p.pool.Exec(ctx, `UPDATE api_tokens SET revoked_at = $1 WHERE id = $2`, time.Now(), tokenID)
	return err
}

func (p *PostgresAppStore) TouchAPIToken(ctx context.Context, tokenID string) error {
	_, err := p.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = $1 WHERE id = $2`, time.Now(), tokenID)
	return err
}

func (p *PostgresAppStore) InsertAgentToken(ctx context.Context, token *control.AgentToken) error {
	scopes, _ := json.Marshal(token.Scopes)
	_, err := p.pool.Exec(ctx, `
		INSERT INTO agent_tokens_new (id, org_id, agent_id, name, token_hash, scopes, expires_at, last_used_at, created_at, revoked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, token.ID, token.OrgID, token.AgentID, token.Name, token.TokenHash, string(scopes),
		toTime(token.ExpiresAt), ptrToTime(token.LastUsedAt), toTime(token.CreatedAt), ptrToTime(token.RevokedAt))
	return err
}

func (p *PostgresAppStore) GetAgentTokenByHash(ctx context.Context, hash string) (*control.AgentToken, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, agent_id, name, token_hash, scopes, expires_at, last_used_at, created_at, revoked_at
		FROM agent_tokens_new WHERE token_hash = $1
	`, []byte(hash))
	return scanPostgresAgentToken(row)
}

func (p *PostgresAppStore) GetAgentToken(ctx context.Context, id string) (*control.AgentToken, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT id, org_id, agent_id, name, token_hash, scopes, expires_at, last_used_at, created_at, revoked_at
		FROM agent_tokens_new WHERE id = $1
	`, id)
	return scanPostgresAgentToken(row)
}

func (p *PostgresAppStore) ListAgentTokens(ctx context.Context, agentID string, page store.Page) (store.PageResult[*control.AgentToken], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, org_id, agent_id, name, token_hash, scopes, expires_at, last_used_at, created_at, revoked_at
		FROM agent_tokens_new WHERE agent_id = $1 ORDER BY created_at DESC
	`, agentID)
	if err != nil {
		return store.PageResult[*control.AgentToken]{}, err
	}
	defer rows.Close()

	var items []*control.AgentToken
	for rows.Next() {
		item, err := scanPostgresAgentToken(rows)
		if err != nil {
			return store.PageResult[*control.AgentToken]{}, err
		}
		items = append(items, item)
	}
	return store.Paginate(items, page), rows.Err()
}

func (p *PostgresAppStore) RevokeAgentToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	_, err := p.pool.Exec(ctx, `UPDATE agent_tokens_new SET revoked_at = $1 WHERE id = $2`, time.Now(), tokenID)
	return err
}

func (p *PostgresAppStore) TouchAgentToken(ctx context.Context, tokenID string) error {
	_, err := p.pool.Exec(ctx, `UPDATE agent_tokens_new SET last_used_at = $1 WHERE id = $2`, time.Now(), tokenID)
	return err
}

func scanPostgresSession(scanner postgresScanner) (*control.Session, error) {
	var s control.Session
	var expiresAt, createdAt time.Time
	var lastUsedAt, revokedAt *time.Time
	err := scanner.Scan(&s.ID, &s.OrgID, &s.UserID, &s.TokenHash, &expiresAt, &createdAt, &lastUsedAt, &s.UserAgent, &s.IP, &revokedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.ExpiresAt = expiresAt.UnixMilli()
	s.CreatedAt = createdAt.UnixMilli()
	s.LastUsedAt = timeToMillisPtr(lastUsedAt)
	s.RevokedAt = timeToMillisPtr(revokedAt)
	return &s, nil
}

func scanPostgresAPIToken(scanner postgresScanner) (*control.APIToken, error) {
	var t control.APIToken
	var scopes string
	var expiresAt, lastUsedAt, revokedAt *time.Time
	var createdAt time.Time
	err := scanner.Scan(&t.ID, &t.OrgID, &t.UserID, &t.Name, &t.TokenHash, &scopes, &expiresAt, &lastUsedAt, &revokedAt, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopes), &t.Scopes)
	t.ExpiresAt = timeToMillisPtr(expiresAt)
	t.LastUsedAt = timeToMillisPtr(lastUsedAt)
	t.RevokedAt = timeToMillisPtr(revokedAt)
	t.CreatedAt = createdAt.UnixMilli()
	return &t, nil
}

func scanPostgresAgentToken(scanner postgresScanner) (*control.AgentToken, error) {
	var t control.AgentToken
	var scopes string
	var expiresAt, createdAt time.Time
	var lastUsedAt, revokedAt *time.Time
	err := scanner.Scan(&t.ID, &t.OrgID, &t.AgentID, &t.Name, &t.TokenHash, &scopes, &expiresAt, &lastUsedAt, &createdAt, &revokedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopes), &t.Scopes)
	t.ExpiresAt = expiresAt.UnixMilli()
	t.LastUsedAt = timeToMillisPtr(lastUsedAt)
	t.CreatedAt = createdAt.UnixMilli()
	t.RevokedAt = timeToMillisPtr(revokedAt)
	return &t, nil
}

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
func (p *PostgresAppStore) UpdatePolicy(ctx context.Context, id string, u *control.PolicyUpdate) error {
	if u == nil {
		return nil
	}
	query := `UPDATE policies SET updated_at = NOW()`
	args := []any{}
	if u.Description != nil {
		query += `, description = $1`
		args = append(args, *u.Description)
	}
	if len(u.AllowedConnectors) > 0 {
		query += fmt.Sprintf(`, allowed_connectors = $%d`, len(args)+1)
		args = append(args, u.AllowedConnectors)
	}
	if len(u.AllowedMethods) > 0 {
		query += fmt.Sprintf(`, allowed_methods = $%d`, len(args)+1)
		args = append(args, u.AllowedMethods)
	}
	if u.CasbinDocument != nil {
		if *u.CasbinDocument == "" {
			query += `, casbin_document = NULL`
		} else {
			query += fmt.Sprintf(`, casbin_document = $%d`, len(args)+1)
			args = append(args, *u.CasbinDocument)
		}
	}
	if u.ClearBudgetCap {
		query += `, budget_cap_amount_nanos = 0`
	} else if u.BudgetCap != nil {
		query += fmt.Sprintf(`, budget_cap_amount_nanos = $%d`, len(args)+1)
		args = append(args, amountToDB(*u.BudgetCap))
	}
	if u.Config != nil {
		query += fmt.Sprintf(`, config = $%d`, len(args)+1)
		args = append(args, u.Config)
	}
	query += fmt.Sprintf(` WHERE id = $%d`, len(args)+1)
	args = append(args, id)
	_, err := p.pool.Exec(ctx, query, args...)
	return err
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
	return scanPostgresCredential(p.pool.QueryRow(ctx, `
		SELECT id, project_id, env_id, connector_type, account_id, label, description,
		       source_type, encrypted_blob, key_hash, wrapping_key_id,
		       secret_ref, secret_version, status, version,
		       expires_at, rotated_at, rotated_by, last_used_at, last_validated_at,
		       created_by, created_at, updated_at, revoked_at, revoked_by, revoke_reason
		FROM credentials WHERE id = $1
	`, id))
}

func (p *PostgresAppStore) StoreCredential(ctx context.Context, c *control.ConnectorCredential) error {
	sourceType := c.SourceType
	if sourceType == "" && c.Source != "" {
		sourceType = string(c.Source)
	}
	secretRef := c.SecretRef
	if secretRef == "" && (sourceType == string(control.CredentialSourceEnv) || c.Source == control.CredentialSourceEnv) {
		secretRef = c.EnvVar
	}
	if secretRef == "" && (sourceType == string(control.CredentialSourceVault) || c.Source == control.CredentialSourceVault) {
		secretRef = c.VaultRef
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO credentials (
			id, project_id, env_id, connector_type, account_id, label, description,
			source_type, encrypted_blob, key_hash, wrapping_key_id,
			secret_ref, secret_version, status, version,
			expires_at, rotated_at, rotated_by, created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
		ON CONFLICT (id) DO UPDATE SET
			label = excluded.label,
			description = excluded.description,
			encrypted_blob = excluded.encrypted_blob,
			key_hash = excluded.key_hash,
			wrapping_key_id = excluded.wrapping_key_id,
			secret_ref = excluded.secret_ref,
			secret_version = excluded.secret_version,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, c.ID, c.ProjectID, c.EnvID, c.ConnectorType, c.AccountID, c.Label, c.Description,
		sourceType, c.EncryptedBlob, c.KeyHash, c.WrappingKeyID, secretRef, c.SecretVersion,
		c.Status, c.Version, ptrToTime(c.ExpiresAt), ptrToTime(c.RotatedAt), c.RotatedBy,
		c.CreatedBy, toTime(c.CreatedAt), toTime(c.UpdatedAt))
	return err
}

func (p *PostgresAppStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM credentials WHERE id = $1`, id)
	return err
}

func (p *PostgresAppStore) ListCredentials(ctx context.Context, envID string, page store.Page) (store.PageResult[*control.ConnectorCredential], error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, project_id, env_id, connector_type, account_id, label, description,
		       source_type, encrypted_blob, key_hash, wrapping_key_id,
		       secret_ref, secret_version, status, version,
		       expires_at, rotated_at, rotated_by, last_used_at, last_validated_at,
		       created_by, created_at, updated_at, revoked_at, revoked_by, revoke_reason
		FROM credentials WHERE env_id = $1 AND status = 'active' ORDER BY created_at DESC
	`, envID)
	if err != nil {
		return store.PageResult[*control.ConnectorCredential]{}, err
	}
	defer rows.Close()

	var items []*control.ConnectorCredential
	for rows.Next() {
		c, err := scanPostgresCredential(rows)
		if err != nil {
			return store.PageResult[*control.ConnectorCredential]{}, err
		}
		items = append(items, c)
	}
	return store.Paginate(items, page), rows.Err()
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
	label := filter.Label
	if label == "" {
		label = "primary"
	}
	return scanPostgresCredential(p.pool.QueryRow(ctx, `
		SELECT id, project_id, env_id, connector_type, account_id, label, description,
		       source_type, encrypted_blob, key_hash, wrapping_key_id,
		       secret_ref, secret_version, status, version,
		       expires_at, rotated_at, rotated_by, last_used_at, last_validated_at,
		       created_by, created_at, updated_at, revoked_at, revoked_by, revoke_reason
		FROM credentials
		WHERE env_id = $1 AND connector_type = $2 AND status = 'active'
		ORDER BY CASE WHEN label = $3 THEN 0 ELSE 1 END, created_at DESC
		LIMIT 1
	`, filter.EnvID, filter.ConnectorType, label))
}

func (p *PostgresAppStore) RotateCredential(ctx context.Context, id string, newBlob []byte, wrappingKeyID, rotatedBy string) error {
	now := time.Now()
	_, err := p.pool.Exec(ctx, `
		UPDATE credentials SET
			encrypted_blob = $1,
			wrapping_key_id = $2,
			rotated_at = $3,
			rotated_by = $4,
			version = version + 1,
			updated_at = $3
		WHERE id = $5
	`, newBlob, wrappingKeyID, now, rotatedBy, id)
	return err
}

func (p *PostgresAppStore) RevokeCredential(ctx context.Context, id, revokedBy, reason string) error {
	now := time.Now()
	_, err := p.pool.Exec(ctx, `
		UPDATE credentials SET
			status = 'revoked',
			revoked_at = $1,
			revoked_by = $2,
			revoke_reason = $3,
			updated_at = $1
		WHERE id = $4
	`, now, revokedBy, reason, id)
	return err
}

func (p *PostgresAppStore) TouchCredential(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `UPDATE credentials SET last_used_at = $1 WHERE id = $2`, time.Now(), id)
	return err
}

func scanPostgresCredential(scanner postgresScanner) (*control.ConnectorCredential, error) {
	var c control.ConnectorCredential
	var expiresAt, rotatedAt, lastUsedAt, lastValidatedAt, revokedAt *time.Time
	var createdAt, updatedAt time.Time
	err := scanner.Scan(
		&c.ID, &c.ProjectID, &c.EnvID, &c.ConnectorType, &c.AccountID, &c.Label, &c.Description,
		&c.SourceType, &c.EncryptedBlob, &c.KeyHash, &c.WrappingKeyID,
		&c.SecretRef, &c.SecretVersion, &c.Status, &c.Version,
		&expiresAt, &rotatedAt, &c.RotatedBy, &lastUsedAt, &lastValidatedAt,
		&c.CreatedBy, &createdAt, &updatedAt, &revokedAt, &c.RevokedBy, &c.RevokeReason,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.ExpiresAt = timeToMillisPtr(expiresAt)
	c.RotatedAt = timeToMillisPtr(rotatedAt)
	c.LastUsedAt = timeToMillisPtr(lastUsedAt)
	c.LastValidatedAt = timeToMillisPtr(lastValidatedAt)
	c.CreatedAt = createdAt.UnixMilli()
	c.UpdatedAt = updatedAt.UnixMilli()
	c.RevokedAt = timeToMillisPtr(revokedAt)
	hydratePostgresCredentialSource(&c)
	return &c, nil
}

func hydratePostgresCredentialSource(c *control.ConnectorCredential) {
	if c == nil {
		return
	}
	if c.Source == "" && c.SourceType != "" {
		c.Source = control.CredentialSource(c.SourceType)
	}
	switch c.Source {
	case control.CredentialSourceEnv:
		c.EnvVar = c.SecretRef
	case control.CredentialSourceVault:
		c.VaultRef = c.SecretRef
	}
}

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
func jsonString(v *[]byte) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}
func ptrToTime(ms *int64) *time.Time {
	if ms == nil {
		return nil
	}
	t := time.UnixMilli(*ms)
	return &t
}
func timeToMillisPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
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
