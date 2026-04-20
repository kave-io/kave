// Package sqlite provides a SQLite-backed AppStore implementation.
// CGO_ENABLED=1 required — uses mattn/go-sqlite3
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/db/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteAppStore implements store.AppStore using SQLite with WAL mode.
type SQLiteAppStore struct {
	db   *sql.DB
	path string
}

// New creates a new SQLite app store with the given file path.
func New(path string) (*SQLiteAppStore, error) {
	dsn := "file:" + path + "?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: ping database: %w", err)
	}
	s := &SQLiteAppStore{db: db, path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteAppStore) Close() error { return s.db.Close() }

func (s *SQLiteAppStore) Migrate(ctx context.Context) error { return sqlite.Migrate(ctx, s.db) }

func (s *SQLiteAppStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *SQLiteAppStore) Stats(ctx context.Context) (map[string]any, error) {
	stats := map[string]any{
		"backend": "sqlite",
		"path":    s.path,
	}
	if info, err := os.Stat(s.path); err == nil {
		stats["size_bytes"] = info.Size()
	}
	tables := []string{
		"orgs", "users", "memberships", "projects", "environments",
		"policies", "agents", "budgets", "runs", "actions",
		"price_book", "fx_rates", "fx_currencies", "agent_tokens", "credentials",
	}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var n int64
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			continue
		}
		counts[table] = n
	}
	stats["tables"] = counts
	return stats, nil
}

// ── OrgStore ──────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateOrg(ctx context.Context, o *control.Organization) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO orgs (id, name, slug, plan, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		o.ID, o.Name, o.Slug, o.Plan, o.CreatedAt, o.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetOrg(ctx context.Context, id string) (*control.Organization, error) {
	var o control.Organization
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, plan, created_at, updated_at FROM orgs WHERE id = ?`, id).
		Scan(&o.ID, &o.Name, &o.Slug, &o.Plan, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &o, err
}

func (s *SQLiteAppStore) GetOrgBySlug(ctx context.Context, slug string) (*control.Organization, error) {
	var o control.Organization
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, plan, created_at, updated_at FROM orgs WHERE slug = ?`, slug).
		Scan(&o.ID, &o.Name, &o.Slug, &o.Plan, &o.CreatedAt, &o.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &o, err
}

func (s *SQLiteAppStore) ListOrgs(ctx context.Context, page store.Page) (store.PageResult[*control.Organization], error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, slug, plan, created_at, updated_at FROM orgs ORDER BY created_at ASC`)
	if err != nil {
		return store.PageResult[*control.Organization]{}, err
	}
	defer rows.Close()

	var items []*control.Organization
	for rows.Next() {
		var o control.Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.Plan, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return store.PageResult[*control.Organization]{}, err
		}
		items = append(items, &o)
	}
	return store.Paginate(items, page), rows.Err()
}

// ── UserStore ─────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateUser(ctx context.Context, u *control.User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, org_id, email, name, password_hash, status, last_login_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.OrgID, u.Email, u.Name, u.PasswordHash, u.Status, u.LastLoginAt, u.CreatedAt, u.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetUser(ctx context.Context, id string) (*control.User, error) {
	var u control.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, email, name, password_hash, status, last_login_at, created_at, updated_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (s *SQLiteAppStore) GetUserByEmail(ctx context.Context, orgID, email string) (*control.User, error) {
	var u control.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, email, name, password_hash, status, last_login_at, created_at, updated_at FROM users WHERE org_id = ? AND email = ?`, orgID, email).
		Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (s *SQLiteAppStore) UpdateUser(ctx context.Context, id string, update *control.UserUpdate) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET
			name         = COALESCE(?, name),
			status       = COALESCE(?, status),
			last_login_at = COALESCE(?, last_login_at),
			updated_at   = ?
		WHERE id = ?`,
		update.Name, update.Status, update.LastLoginAt, now, id)
	return err
}

// ── MembershipStore ───────────────────────────────────────────────────────────

func (s *SQLiteAppStore) AddMember(ctx context.Context, m *control.Membership) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memberships (id, org_id, user_id, role, invited_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, m.OrgID, m.UserID, m.Role, m.InvitedBy, m.CreatedAt)
	return err
}

func (s *SQLiteAppStore) GetMembership(ctx context.Context, orgID, userID string) (*control.Membership, error) {
	var m control.Membership
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, user_id, role, invited_by, created_at FROM memberships WHERE org_id = ? AND user_id = ?`, orgID, userID).
		Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.InvitedBy, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &m, err
}

func (s *SQLiteAppStore) ListMembers(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Membership], error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, user_id, role, invited_by, created_at FROM memberships WHERE org_id = ? ORDER BY created_at ASC`,
		orgID)
	if err != nil {
		return store.PageResult[*control.Membership]{}, err
	}
	defer rows.Close()

	var items []*control.Membership
	for rows.Next() {
		var m control.Membership
		if err := rows.Scan(&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.InvitedBy, &m.CreatedAt); err != nil {
			return store.PageResult[*control.Membership]{}, err
		}
		items = append(items, &m)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) RemoveMember(ctx context.Context, orgID, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM memberships WHERE org_id = ? AND user_id = ?`, orgID, userID)
	return err
}

// ── ProjectStore ──────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateProject(ctx context.Context, p *control.Project) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (id, org_id, name, slug, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.OrgID, p.Name, p.Slug, p.Description, p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetProject(ctx context.Context, id string) (*control.Project, error) {
	var p control.Project
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, name, slug, description, created_at, updated_at FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

func (s *SQLiteAppStore) ListProjects(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Project], error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, name, slug, description, created_at, updated_at FROM projects WHERE org_id = ? ORDER BY name ASC`,
		orgID)
	if err != nil {
		return store.PageResult[*control.Project]{}, err
	}
	defer rows.Close()

	var items []*control.Project
	for rows.Next() {
		var p control.Project
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return store.PageResult[*control.Project]{}, err
		}
		items = append(items, &p)
	}
	return store.Paginate(items, page), rows.Err()
}

// ── EnvironmentStore ──────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateEnvironment(ctx context.Context, e *control.Environment) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO environments (id, project_id, name, slug, type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.ProjectID, e.Name, e.Slug, e.Type, e.CreatedAt, e.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetEnvironment(ctx context.Context, id string) (*control.Environment, error) {
	var e control.Environment
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, slug, type, created_at, updated_at FROM environments WHERE id = ?`, id).
		Scan(&e.ID, &e.ProjectID, &e.Name, &e.Slug, &e.Type, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &e, err
}

func (s *SQLiteAppStore) GetEnvironmentBySlug(ctx context.Context, projectID, slug string) (*control.Environment, error) {
	var e control.Environment
	err := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, name, slug, type, created_at, updated_at FROM environments WHERE project_id = ? AND slug = ?`, projectID, slug).
		Scan(&e.ID, &e.ProjectID, &e.Name, &e.Slug, &e.Type, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &e, err
}

func (s *SQLiteAppStore) ListEnvironments(ctx context.Context, projectID string, page store.Page) (store.PageResult[*control.Environment], error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, name, slug, type, created_at, updated_at FROM environments WHERE project_id = ? ORDER BY name ASC`,
		projectID)
	if err != nil {
		return store.PageResult[*control.Environment]{}, err
	}
	defer rows.Close()

	var items []*control.Environment
	for rows.Next() {
		var e control.Environment
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Slug, &e.Type, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return store.PageResult[*control.Environment]{}, err
		}
		items = append(items, &e)
	}
	return store.Paginate(items, page), rows.Err()
}

// ── PolicyStore ───────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreatePolicy(ctx context.Context, p *control.PolicyRecord) error {
	typesJSON, _ := json.Marshal(p.AllowedTypes)
	connectorsJSON, _ := json.Marshal(p.AllowedConnectors)
	methodsJSON, _ := json.Marshal(p.AllowedMethods)
	configJSON, _ := json.Marshal(p.Config)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO policies (
			id, project_id, env_id, name, description,
			allowed_types, allowed_connectors, allowed_methods,
			budget_cap_nanos, budget_period, budget_behavior,
			trace_input, trace_output, retention_days, config,
			version, mode, status, created_by, updated_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ProjectID, p.EnvID, p.Name, p.Description,
		string(typesJSON), string(connectorsJSON), string(methodsJSON),
		int64(p.BudgetCap), p.BudgetPeriod, p.BudgetBehavior,
		boolToInt(p.TraceInput), boolToInt(p.TraceOutput), p.RetentionDays, string(configJSON),
		p.Version, p.Mode, string(p.Status), p.CreatedBy, p.UpdatedBy, p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetPolicy(ctx context.Context, id string) (*control.PolicyRecord, error) {
	return s.scanPolicy(s.db.QueryRowContext(ctx, `
		SELECT id, project_id, env_id, name, description,
		       allowed_types, allowed_connectors, allowed_methods,
		       budget_cap_nanos, budget_period, budget_behavior,
		       trace_input, trace_output, retention_days, config,
		       version, mode, status, created_by, updated_by, created_at, updated_at
		FROM policies WHERE id = ?`, id))
}

func (s *SQLiteAppStore) GetAgentPolicy(ctx context.Context, agentID string) (*control.PolicyRecord, error) {
	var policyID *string
	err := s.db.QueryRowContext(ctx, `SELECT policy_id FROM agents WHERE id = ?`, agentID).Scan(&policyID)
	if err != nil || policyID == nil {
		return nil, err
	}
	return s.GetPolicy(ctx, *policyID)
}

func (s *SQLiteAppStore) ListPolicies(ctx context.Context, envID string, page store.Page) (store.PageResult[*control.PolicyRecord], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, env_id, name, description,
		       allowed_types, allowed_connectors, allowed_methods,
		       budget_cap_nanos, budget_period, budget_behavior,
		       trace_input, trace_output, retention_days, config,
		       version, mode, status, created_by, updated_by, created_at, updated_at
		FROM policies WHERE env_id = ? ORDER BY name ASC`, envID)
	if err != nil {
		return store.PageResult[*control.PolicyRecord]{}, err
	}
	defer rows.Close()

	var items []*control.PolicyRecord
	for rows.Next() {
		p, err := s.scanPolicyRow(rows)
		if err != nil {
			return store.PageResult[*control.PolicyRecord]{}, err
		}
		items = append(items, p)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) UpdatePolicy(ctx context.Context, id string, u *control.PolicyUpdate) error {
	if u == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	query := `UPDATE policies SET updated_at = ?, version = version + 1`
	args := []any{now}
	if u.Description != nil {
		query += `, description = ?`
		args = append(args, *u.Description)
	}
	if len(u.AllowedTypes) > 0 {
		b, _ := json.Marshal(u.AllowedTypes)
		query += `, allowed_types = ?`
		args = append(args, string(b))
	}
	if len(u.AllowedConnectors) > 0 {
		b, _ := json.Marshal(u.AllowedConnectors)
		query += `, allowed_connectors = ?`
		args = append(args, string(b))
	}
	if len(u.AllowedMethods) > 0 {
		b, _ := json.Marshal(u.AllowedMethods)
		query += `, allowed_methods = ?`
		args = append(args, string(b))
	}
	if u.ClearBudgetCap {
		query += `, budget_cap_nanos = 0`
	} else if u.BudgetCap != nil {
		query += `, budget_cap_nanos = ?`
		args = append(args, int64(*u.BudgetCap))
	}
	if u.BudgetPeriod != nil {
		query += `, budget_period = ?`
		args = append(args, *u.BudgetPeriod)
	}
	if u.BudgetBehavior != nil {
		query += `, budget_behavior = ?`
		args = append(args, *u.BudgetBehavior)
	}
	if u.TraceInput != nil {
		query += `, trace_input = ?`
		args = append(args, boolToInt(*u.TraceInput))
	}
	if u.TraceOutput != nil {
		query += `, trace_output = ?`
		args = append(args, boolToInt(*u.TraceOutput))
	}
	if u.RetentionDays != nil {
		query += `, retention_days = ?`
		args = append(args, *u.RetentionDays)
	}
	if u.Config != nil {
		b, _ := json.Marshal(u.Config)
		query += `, config = ?`
		args = append(args, string(b))
	}
	if u.Mode != nil {
		query += `, mode = ?`
		args = append(args, *u.Mode)
	}
	if u.Status != nil {
		query += `, status = ?`
		args = append(args, *u.Status)
	}
	if u.UpdatedBy != nil {
		query += `, updated_by = ?`
		args = append(args, *u.UpdatedBy)
	}
	query += ` WHERE id = ?`
	args = append(args, id)
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *SQLiteAppStore) DeletePolicy(ctx context.Context, id string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE policies SET status = 'archived', updated_at = ? WHERE id = ?`, now, id)
	return err
}

func (s *SQLiteAppStore) scanPolicy(row *sql.Row) (*control.PolicyRecord, error) {
	var p control.PolicyRecord
	var typesJSON, connectorsJSON, methodsJSON, configJSON string
	var budgetCapNanos int64
	var traceInput, traceOutput int
	var status string
	err := row.Scan(
		&p.ID, &p.ProjectID, &p.EnvID, &p.Name, &p.Description,
		&typesJSON, &connectorsJSON, &methodsJSON,
		&budgetCapNanos, &p.BudgetPeriod, &p.BudgetBehavior,
		&traceInput, &traceOutput, &p.RetentionDays, &configJSON,
		&p.Version, &p.Mode, &status, &p.CreatedBy, &p.UpdatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.BudgetCap = money.Amount(budgetCapNanos)
	p.TraceInput = traceInput == 1
	p.TraceOutput = traceOutput == 1
	p.Status = status
	_ = json.Unmarshal([]byte(typesJSON), &p.AllowedTypes)
	_ = json.Unmarshal([]byte(connectorsJSON), &p.AllowedConnectors)
	_ = json.Unmarshal([]byte(methodsJSON), &p.AllowedMethods)
	_ = json.Unmarshal([]byte(configJSON), &p.Config)
	return &p, nil
}

func (s *SQLiteAppStore) scanPolicyRow(rows *sql.Rows) (*control.PolicyRecord, error) {
	var p control.PolicyRecord
	var typesJSON, connectorsJSON, methodsJSON, configJSON, status string
	var budgetCapNanos int64
	var traceInput, traceOutput int
	if err := rows.Scan(
		&p.ID, &p.ProjectID, &p.EnvID, &p.Name, &p.Description,
		&typesJSON, &connectorsJSON, &methodsJSON,
		&budgetCapNanos, &p.BudgetPeriod, &p.BudgetBehavior,
		&traceInput, &traceOutput, &p.RetentionDays, &configJSON,
		&p.Version, &p.Mode, &status, &p.CreatedBy, &p.UpdatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.BudgetCap = money.Amount(budgetCapNanos)
	p.TraceInput = traceInput == 1
	p.TraceOutput = traceOutput == 1
	p.Status = status
	_ = json.Unmarshal([]byte(typesJSON), &p.AllowedTypes)
	_ = json.Unmarshal([]byte(connectorsJSON), &p.AllowedConnectors)
	_ = json.Unmarshal([]byte(methodsJSON), &p.AllowedMethods)
	_ = json.Unmarshal([]byte(configJSON), &p.Config)
	return &p, nil
}

// ── AgentStore ────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateAgent(ctx context.Context, a *control.Agent) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	var budgetNanos *int64
	if a.MonthlyBudget != nil {
		v := int64(*a.MonthlyBudget)
		budgetNanos = &v
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agents (id, project_id, env_id, name, description, policy_id, monthly_budget_nanos, status, metadata, created_by, updated_by, deleted_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ProjectID, a.EnvID, a.Name, a.Description, a.PolicyID, budgetNanos,
		a.Status, string(metaJSON), a.CreatedBy, a.UpdatedBy, a.DeletedAt, a.CreatedAt, a.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetAgentByID(ctx context.Context, id string) (*control.Agent, error) {
	return s.scanAgent(s.db.QueryRowContext(ctx, `
		SELECT id, project_id, env_id, name, description, policy_id, monthly_budget_nanos, status, metadata, created_by, updated_by, deleted_at, created_at, updated_at
		FROM agents WHERE id = ?`, id))
}

func (s *SQLiteAppStore) GetAgentByName(ctx context.Context, envID, name string) (*control.Agent, error) {
	return s.scanAgent(s.db.QueryRowContext(ctx, `
		SELECT id, project_id, env_id, name, description, policy_id, monthly_budget_nanos, status, metadata, created_by, updated_by, deleted_at, created_at, updated_at
		FROM agents WHERE env_id = ? AND name = ? AND deleted_at IS NULL`, envID, name))
}

func (s *SQLiteAppStore) UpdateAgent(ctx context.Context, id string, update *control.AgentUpdate) error {
	now := time.Now().UnixMilli()
	var budgetNanos *int64
	if update.MonthlyBudget != nil {
		v := int64(*update.MonthlyBudget)
		budgetNanos = &v
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE agents SET
			description          = COALESCE(?, description),
			policy_id            = COALESCE(?, policy_id),
			monthly_budget_nanos = COALESCE(?, monthly_budget_nanos),
			status               = COALESCE(?, status),
			metadata             = COALESCE(?, metadata),
			updated_by           = COALESCE(?, updated_by),
			updated_at           = ?
		WHERE id = ?`,
		update.Description, update.PolicyID, budgetNanos,
		update.Status, marshalMetadata(update.Metadata), update.UpdatedBy, now, id)
	return err
}

func (s *SQLiteAppStore) ListAgents(ctx context.Context, envID string, page store.Page) (store.PageResult[*control.Agent], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, env_id, name, description, policy_id, monthly_budget_nanos, status, metadata, created_by, updated_by, deleted_at, created_at, updated_at
		FROM agents WHERE env_id = ? AND deleted_at IS NULL ORDER BY name ASC`, envID)
	if err != nil {
		return store.PageResult[*control.Agent]{}, err
	}
	defer rows.Close()

	var items []*control.Agent
	for rows.Next() {
		a, err := s.scanAgentRow(rows)
		if err != nil {
			return store.PageResult[*control.Agent]{}, err
		}
		items = append(items, a)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) DeleteAgent(ctx context.Context, id, deletedBy string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET deleted_at = ?, updated_by = ?, updated_at = ? WHERE id = ?`,
		now, deletedBy, now, id)
	return err
}

func (s *SQLiteAppStore) RestoreAgent(ctx context.Context, id, restoredBy string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET deleted_at = NULL, updated_by = ?, updated_at = ? WHERE id = ?`,
		restoredBy, now, id)
	return err
}

// ── BudgetStore ──────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateBudget(ctx context.Context, b *control.Budget) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO budgets (id, agent_id, hard_cap_nanos, soft_cap_nanos, period, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			hard_cap_nanos = excluded.hard_cap_nanos,
			soft_cap_nanos = excluded.soft_cap_nanos,
			period = excluded.period,
			updated_at = excluded.updated_at`,
		b.ID, b.AgentID, int64(b.HardCap), int64(b.SoftCap), b.Period, b.CreatedAt, b.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetBudget(ctx context.Context, agentID string) (*control.Budget, error) {
	var b control.Budget
	var hardCapNanos, softCapNanos int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, hard_cap_nanos, soft_cap_nanos, period, created_at, updated_at
		FROM budgets WHERE agent_id = ?`, agentID).Scan(
		&b.ID, &b.AgentID, &hardCapNanos, &softCapNanos, &b.Period, &b.CreatedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.HardCap = money.Amount(hardCapNanos)
	b.SoftCap = money.Amount(softCapNanos)
	return &b, nil
}

func (s *SQLiteAppStore) DeleteBudget(ctx context.Context, agentID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM budgets WHERE agent_id = ?`, agentID)
	return err
}

func (s *SQLiteAppStore) scanAgent(row *sql.Row) (*control.Agent, error) {
	var a control.Agent
	var metaJSON string
	var budgetNanos *int64
	err := row.Scan(
		&a.ID, &a.ProjectID, &a.EnvID, &a.Name, &a.Description,
		&a.PolicyID, &budgetNanos, &a.Status, &metaJSON,
		&a.CreatedBy, &a.UpdatedBy, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if budgetNanos != nil {
		v := money.Amount(*budgetNanos)
		a.MonthlyBudget = &v
	}
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	if a.Metadata == nil {
		a.Metadata = make(map[string]any)
	}
	return &a, nil
}

func (s *SQLiteAppStore) scanAgentRow(rows *sql.Rows) (*control.Agent, error) {
	var a control.Agent
	var metaJSON string
	var budgetNanos *int64
	if err := rows.Scan(
		&a.ID, &a.ProjectID, &a.EnvID, &a.Name, &a.Description,
		&a.PolicyID, &budgetNanos, &a.Status, &metaJSON,
		&a.CreatedBy, &a.UpdatedBy, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	if budgetNanos != nil {
		v := money.Amount(*budgetNanos)
		a.MonthlyBudget = &v
	}
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	if a.Metadata == nil {
		a.Metadata = make(map[string]any)
	}
	return &a, nil
}

// ── RunStore ──────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateRun(ctx context.Context, r *runtimemodel.RunRecord) error {
	metaJSON, _ := json.Marshal(r.Metadata)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runs (
			id, project_id, env_id, agent_id, policy_id, name, status,
			budget_cap_nanos, spent_nanos, metadata, error_message,
			trigger_type, trigger_id, correlation_id, session_id, idempotency_key,
			started_at, ended_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.EnvID, r.AgentID, r.PolicyID, r.Name, r.Status,
		int64(r.BudgetCap), int64(r.Spent), string(metaJSON), r.ErrorMessage,
		r.TriggerType, r.TriggerID, r.CorrelationID, r.SessionID, r.IdempotencyKey,
		r.StartedAt, r.EndedAt, r.CreatedAt, r.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetRunByID(ctx context.Context, id string) (*runtimemodel.RunRecord, error) {
	return s.scanRun(s.db.QueryRowContext(ctx, `
		SELECT id, project_id, env_id, agent_id, policy_id, name, status,
		       budget_cap_nanos, spent_nanos, metadata, error_message,
		       trigger_type, trigger_id, correlation_id, session_id, idempotency_key,
		       started_at, ended_at, created_at, updated_at
		FROM runs WHERE id = ?`, id))
}

func (s *SQLiteAppStore) GetRunByIdempotencyKey(ctx context.Context, envID, key string) (*runtimemodel.RunRecord, error) {
	return s.scanRun(s.db.QueryRowContext(ctx, `
		SELECT id, project_id, env_id, agent_id, policy_id, name, status,
		       budget_cap_nanos, spent_nanos, metadata, error_message,
		       trigger_type, trigger_id, correlation_id, session_id, idempotency_key,
		       started_at, ended_at, created_at, updated_at
		FROM runs WHERE env_id = ? AND idempotency_key = ?`, envID, key))
}

func (s *SQLiteAppStore) UpdateRun(ctx context.Context, id string, update *runtimemodel.RunUpdate) error {
	now := time.Now().UnixMilli()
	var spentNanos *int64
	if update.Spent != nil {
		v := int64(*update.Spent)
		spentNanos = &v
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE runs SET
			status        = COALESCE(?, status),
			spent_nanos   = COALESCE(?, spent_nanos),
			error_message = COALESCE(?, error_message),
			ended_at      = COALESCE(?, ended_at),
			metadata      = COALESCE(?, metadata),
			updated_at    = ?
		WHERE id = ?`,
		update.Status, spentNanos, update.ErrorMessage, update.EndedAt,
		marshalMetadata(update.Metadata), now, id)
	return err
}

func (s *SQLiteAppStore) ListRuns(ctx context.Context, filter *runtimemodel.RunFilter, page store.Page) (store.PageResult[*runtimemodel.RunRecord], error) {
	query := `
		SELECT id, project_id, env_id, agent_id, policy_id, name, status,
		       budget_cap_nanos, spent_nanos, metadata, error_message,
		       trigger_type, trigger_id, correlation_id, session_id, idempotency_key,
		       started_at, ended_at, created_at, updated_at
		FROM runs WHERE 1=1`
	var args []any

	if filter.ProjectID != "" {
		query += ` AND project_id = ?`
		args = append(args, filter.ProjectID)
	}
	if filter.EnvID != "" {
		query += ` AND env_id = ?`
		args = append(args, filter.EnvID)
	}
	if filter.AgentID != "" {
		query += ` AND agent_id = ?`
		args = append(args, filter.AgentID)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.FromMs != nil {
		query += ` AND started_at >= ?`
		args = append(args, *filter.FromMs)
	}
	if filter.ToMs != nil {
		query += ` AND started_at <= ?`
		args = append(args, *filter.ToMs)
	}
	query += ` ORDER BY started_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.PageResult[*runtimemodel.RunRecord]{}, err
	}
	defer rows.Close()

	var items []*runtimemodel.RunRecord
	for rows.Next() {
		r, err := s.scanRunRow(rows)
		if err != nil {
			return store.PageResult[*runtimemodel.RunRecord]{}, err
		}
		items = append(items, r)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) scanRun(row *sql.Row) (*runtimemodel.RunRecord, error) {
	var r runtimemodel.RunRecord
	var metaJSON string
	var budgetCapNanos, spentNanos int64
	err := row.Scan(
		&r.ID, &r.ProjectID, &r.EnvID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status,
		&budgetCapNanos, &spentNanos, &metaJSON, &r.ErrorMessage,
		&r.TriggerType, &r.TriggerID, &r.CorrelationID, &r.SessionID, &r.IdempotencyKey,
		&r.StartedAt, &r.EndedAt, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.BudgetCap = money.Amount(budgetCapNanos)
	r.Spent = money.Amount(spentNanos)
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &r.Metadata)
	}
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
	return &r, nil
}

func (s *SQLiteAppStore) scanRunRow(rows *sql.Rows) (*runtimemodel.RunRecord, error) {
	var r runtimemodel.RunRecord
	var metaJSON string
	var budgetCapNanos, spentNanos int64
	if err := rows.Scan(
		&r.ID, &r.ProjectID, &r.EnvID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status,
		&budgetCapNanos, &spentNanos, &metaJSON, &r.ErrorMessage,
		&r.TriggerType, &r.TriggerID, &r.CorrelationID, &r.SessionID, &r.IdempotencyKey,
		&r.StartedAt, &r.EndedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	r.BudgetCap = money.Amount(budgetCapNanos)
	r.Spent = money.Amount(spentNanos)
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &r.Metadata)
	}
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
	return &r, nil
}

// ── ActionStore ───────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateAction(ctx context.Context, a *runtimemodel.ActionRecord) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO actions (
			id, run_id, agent_id, project_id, env_id, parent_id,
			action_type, connector, method, input, output, error,
			started_at, ended_at, depth, seq, status, source,
			metadata, attempt, max_attempts, retry_reason, provider_req_id, external_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.RunID, a.AgentID, a.ProjectID, a.EnvID, a.ParentID,
		a.ActionType, a.Connector, a.Method, a.Input, a.Output, a.Error,
		a.StartedAt, a.EndedAt, a.Depth, a.Seq, a.Status, a.Source,
		string(metaJSON), a.Attempt, a.MaxAttempts, a.RetryReason, a.ProviderReqID, a.ExternalID, a.CreatedAt)
	return err
}

func (s *SQLiteAppStore) GetAction(ctx context.Context, id string) (*runtimemodel.ActionRecord, error) {
	return s.scanAction(s.db.QueryRowContext(ctx, `
		SELECT id, run_id, agent_id, project_id, env_id, parent_id,
		       action_type, connector, method, input, output, error,
		       started_at, ended_at, depth, seq, status, source,
		       metadata, attempt, max_attempts, retry_reason, provider_req_id, external_id, created_at
		FROM actions WHERE id = ?`, id))
}

func (s *SQLiteAppStore) ListActionsByRun(ctx context.Context, runID string, page store.Page) (store.PageResult[*runtimemodel.ActionRecord], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, agent_id, project_id, env_id, parent_id,
		       action_type, connector, method, input, output, error,
		       started_at, ended_at, depth, seq, status, source,
		       metadata, attempt, max_attempts, retry_reason, provider_req_id, external_id, created_at
		FROM actions WHERE run_id = ? ORDER BY created_at ASC`, runID)
	if err != nil {
		return store.PageResult[*runtimemodel.ActionRecord]{}, err
	}
	defer rows.Close()

	var items []*runtimemodel.ActionRecord
	for rows.Next() {
		a, err := s.scanActionRow(rows)
		if err != nil {
			return store.PageResult[*runtimemodel.ActionRecord]{}, err
		}
		items = append(items, a)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) scanAction(row *sql.Row) (*runtimemodel.ActionRecord, error) {
	var a runtimemodel.ActionRecord
	var metaJSON string
	err := row.Scan(
		&a.ID, &a.RunID, &a.AgentID, &a.ProjectID, &a.EnvID, &a.ParentID,
		&a.ActionType, &a.Connector, &a.Method, &a.Input, &a.Output, &a.Error,
		&a.StartedAt, &a.EndedAt, &a.Depth, &a.Seq, &a.Status, &a.Source,
		&metaJSON, &a.Attempt, &a.MaxAttempts, &a.RetryReason, &a.ProviderReqID, &a.ExternalID, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	return &a, nil
}

func (s *SQLiteAppStore) scanActionRow(rows *sql.Rows) (*runtimemodel.ActionRecord, error) {
	var a runtimemodel.ActionRecord
	var metaJSON string
	if err := rows.Scan(
		&a.ID, &a.RunID, &a.AgentID, &a.ProjectID, &a.EnvID, &a.ParentID,
		&a.ActionType, &a.Connector, &a.Method, &a.Input, &a.Output, &a.Error,
		&a.StartedAt, &a.EndedAt, &a.Depth, &a.Seq, &a.Status, &a.Source,
		&metaJSON, &a.Attempt, &a.MaxAttempts, &a.RetryReason, &a.ProviderReqID, &a.ExternalID, &a.CreatedAt); err != nil {
		return nil, err
	}
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	return &a, nil
}

// ── CostStore ─────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) GetPriceBook(ctx context.Context) (*runtimemodel.PriceBook, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT version, provider, match, source, currency,
		       input_per_million_amount_nanos, output_per_million_amount_nanos,
		       cache_read_per_million_amount_nanos, cache_write_per_million_amount_nanos,
		       reasoning_per_million_amount_nanos, audio_input_per_million_amount_nanos,
		       audio_output_per_million_amount_nanos, image_unit_price_amount_nanos,
		       per_request_amount_nanos, per_compute_ms_amount_nanos,
		       per_gb_stored_amount_nanos, per_gb_transferred_amount_nanos,
		       effective_from, effective_to, revision_note
		FROM price_book_entries ORDER BY sort_order ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	book := &runtimemodel.PriceBook{Entries: []runtimemodel.PriceModel{}}
	for rows.Next() {
		var version string
		var entry runtimemodel.PriceModel
		if err := rows.Scan(
			&version, &entry.Provider, &entry.Match, &entry.Source, &entry.Currency,
			&entry.InputPerMillion, &entry.OutputPerMillion,
			&entry.CacheReadPerMillion, &entry.CacheWritePerMillion,
			&entry.ReasoningPerMillion, &entry.AudioInputPerMillion,
			&entry.AudioOutputPerMillion, &entry.ImageUnitPrice,
			&entry.PerRequest, &entry.PerComputeMs,
			&entry.PerGBStored, &entry.PerGBTransferred,
			&entry.EffectiveFrom, &entry.EffectiveTo, &entry.RevisionNote,
		); err != nil {
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

func (s *SQLiteAppStore) SavePriceBook(ctx context.Context, book *runtimemodel.PriceBook) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM price_book_entries`); err != nil {
		return err
	}
	for i, entry := range book.Entries {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO price_book_entries (
				id, version, provider, match, source, currency,
				input_per_million_amount_nanos, output_per_million_amount_nanos,
				cache_read_per_million_amount_nanos, cache_write_per_million_amount_nanos,
				reasoning_per_million_amount_nanos, audio_input_per_million_amount_nanos,
				audio_output_per_million_amount_nanos, image_unit_price_amount_nanos,
				per_request_amount_nanos, per_compute_ms_amount_nanos,
				per_gb_stored_amount_nanos, per_gb_transferred_amount_nanos,
				effective_from, effective_to, revision_note, sort_order
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("%s:%d", book.Version, i), book.Version, entry.Provider, entry.Match, entry.Source, entry.Currency,
			int64(entry.InputPerMillion), int64(entry.OutputPerMillion),
			int64(entry.CacheReadPerMillion), int64(entry.CacheWritePerMillion),
			int64(entry.ReasoningPerMillion), int64(entry.AudioInputPerMillion),
			int64(entry.AudioOutputPerMillion), int64(entry.ImageUnitPrice),
			int64(entry.PerRequest), int64(entry.PerComputeMs),
			int64(entry.PerGBStored), int64(entry.PerGBTransferred),
			entry.EffectiveFrom, entry.EffectiveTo, entry.RevisionNote, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteAppStore) ListFXRates(ctx context.Context) ([]runtimemodel.FXRateRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT base_currency, quote_currency, rate, provider, as_of_date, fetched_at FROM fx_rates ORDER BY base_currency, quote_currency`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []runtimemodel.FXRateRecord
	for rows.Next() {
		var item runtimemodel.FXRateRecord
		if err := rows.Scan(&item.BaseCurrency, &item.QuoteCurrency, &item.Rate, &item.Provider, &item.AsOfDate, &item.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteAppStore) GetFXRate(ctx context.Context, base, quote money.CurrencyCode) (*runtimemodel.FXRateRecord, error) {
	var item runtimemodel.FXRateRecord
	err := s.db.QueryRowContext(ctx, `SELECT base_currency, quote_currency, rate, provider, as_of_date, fetched_at FROM fx_rates WHERE base_currency = ? AND quote_currency = ?`, string(base), string(quote)).
		Scan(&item.BaseCurrency, &item.QuoteCurrency, &item.Rate, &item.Provider, &item.AsOfDate, &item.FetchedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &item, err
}

func (s *SQLiteAppStore) UpsertFXRates(ctx context.Context, rates []runtimemodel.FXRateRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, rate := range rates {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO fx_rates (base_currency, quote_currency, rate, provider, as_of_date, fetched_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(base_currency, quote_currency) DO UPDATE SET
				rate = excluded.rate, provider = excluded.provider,
				as_of_date = excluded.as_of_date, fetched_at = excluded.fetched_at`,
			string(rate.BaseCurrency), string(rate.QuoteCurrency), rate.Rate, rate.Provider, rate.AsOfDate, rate.FetchedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteAppStore) ListFXCurrencies(ctx context.Context) ([]runtimemodel.FXCurrencyRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT code, name, symbol, fetched_at FROM fx_currencies ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []runtimemodel.FXCurrencyRecord
	for rows.Next() {
		var item runtimemodel.FXCurrencyRecord
		if err := rows.Scan(&item.Code, &item.Name, &item.Symbol, &item.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteAppStore) UpsertFXCurrencies(ctx context.Context, currencies []runtimemodel.FXCurrencyRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range currencies {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO fx_currencies (code, name, symbol, fetched_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(code) DO UPDATE SET name = excluded.name, symbol = excluded.symbol, fetched_at = excluded.fetched_at`,
			string(item.Code), item.Name, item.Symbol, item.FetchedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteAppStore) InsertBudgetEntry(ctx context.Context, entry *runtimemodel.BudgetEntry) error {
	metaJSON, _ := json.Marshal(entry.Metadata)
	usageJSON, _ := json.Marshal(entry.UsageDetail)
	snapshotJSON, _ := json.Marshal(entry.PriceSnapshot)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO budget_ledger (
			id, project_id, env_id, policy_id, agent_id, run_id, action_id, span_id,
			connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			reasoning_tokens, audio_input_tokens, audio_output_tokens, image_units,
			request_count, compute_ms, storage_bytes, bandwidth_bytes,
			cost_nanos, price_version, price_snapshot, usage_detail, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.ProjectID, entry.EnvID, entry.PolicyID, entry.AgentID,
		emptyToNil(entry.RunID), entry.ActionID, entry.SpanID,
		entry.Connector, entry.Model,
		entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens,
		entry.ReasoningTokens, entry.AudioInputTokens, entry.AudioOutputTokens, entry.ImageUnits,
		entry.RequestCount, entry.ComputeMs, entry.StorageBytes, entry.BandwidthBytes,
		int64(entry.Cost), emptyToNil(entry.PriceVersion), string(snapshotJSON), string(usageJSON), string(metaJSON), entry.CreatedAt)
	return err
}

func (s *SQLiteAppStore) AddRunSpend(ctx context.Context, runID string, cost money.Amount) error {
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET spent_nanos = spent_nanos + ? WHERE id = ?`, int64(cost), runID)
	return err
}

func (s *SQLiteAppStore) SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (money.Amount, error) {
	var total int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_nanos), 0) FROM budget_ledger WHERE agent_id = ? AND created_at >= ?`,
		agentID, sinceMs).Scan(&total)
	return money.Amount(total), err
}

func (s *SQLiteAppStore) GetSpendReport(ctx context.Context, filter *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	where, args := spendWhere(filter)

	var totalAmount int64
	var maxTime, minTime *int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_nanos), 0), MAX(created_at), MIN(created_at) FROM budget_ledger `+where, args...).
		Scan(&totalAmount, &maxTime, &minTime)
	if err != nil {
		return nil, err
	}

	report := &runtimemodel.SpendReport{
		Total:       money.Amount(totalAmount),
		ByProject:   make(map[string]money.Amount),
		ByEnv:       make(map[string]money.Amount),
		ByPolicy:    make(map[string]money.Amount),
		ByAgent:     make(map[string]money.Amount),
		ByConnector: make(map[string]money.Amount),
		ByModel:     make(map[string]money.Amount),
	}
	if minTime != nil {
		report.PeriodStart = *minTime
	}
	if maxTime != nil {
		report.PeriodEnd = *maxTime
	}

	s.aggregateDim(ctx, report.ByProject, "project_id", where, args)
	s.aggregateDim(ctx, report.ByEnv, "env_id", where, args)
	s.aggregateDim(ctx, report.ByPolicy, "policy_id", where, args)
	s.aggregateDim(ctx, report.ByAgent, "agent_id", where, args)
	s.aggregateDim(ctx, report.ByConnector, "connector", where, args)
	s.aggregateDim(ctx, report.ByModel, "COALESCE(model, '')", where, args)

	return report, nil
}

func (s *SQLiteAppStore) aggregateDim(ctx context.Context, dest map[string]money.Amount, dim, where string, args []any) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+dim+`, SUM(cost_nanos) FROM budget_ledger `+where+` GROUP BY `+dim, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var cost int64
		if err := rows.Scan(&id, &cost); err == nil {
			dest[id] = money.Amount(cost)
		}
	}
}

func spendWhere(f *runtimemodel.SpendFilter) (string, []any) {
	where := "WHERE 1=1"
	var args []any
	if f.ProjectID != "" {
		where += " AND project_id = ?"
		args = append(args, f.ProjectID)
	}
	if f.EnvID != "" {
		where += " AND env_id = ?"
		args = append(args, f.EnvID)
	}
	if f.PolicyID != "" {
		where += " AND policy_id = ?"
		args = append(args, f.PolicyID)
	}
	if f.AgentID != "" {
		where += " AND agent_id = ?"
		args = append(args, f.AgentID)
	}
	if f.Connector != "" {
		where += " AND connector = ?"
		args = append(args, f.Connector)
	}
	if f.Model != "" {
		where += " AND model = ?"
		args = append(args, f.Model)
	}
	if f.FromMs != nil {
		where += " AND created_at >= ?"
		args = append(args, *f.FromMs)
	}
	if f.ToMs != nil {
		where += " AND created_at <= ?"
		args = append(args, *f.ToMs)
	}
	return where, args
}

// ── CredentialStore ───────────────────────────────────────────────────────────

func (s *SQLiteAppStore) GetCredential(ctx context.Context, id string) (*control.ConnectorCredential, error) {
	return s.scanCredential(s.db.QueryRowContext(ctx, `
		SELECT id, project_id, env_id, connector_type, account_id, label, description,
		       source_type, encrypted_blob, key_hash, wrapping_key_id,
		       secret_ref, secret_version, status, version,
		       expires_at, rotated_at, rotated_by, last_used_at, last_validated_at,
		       created_by, created_at, updated_at, revoked_at, revoked_by, revoke_reason
		FROM credentials WHERE id = ?`, id))
}

func (s *SQLiteAppStore) StoreCredential(ctx context.Context, c *control.ConnectorCredential) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO credentials (
			id, project_id, env_id, connector_type, account_id, label, description,
			source_type, encrypted_blob, key_hash, wrapping_key_id,
			secret_ref, secret_version, status, version,
			expires_at, rotated_at, rotated_by, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			label = excluded.label, description = excluded.description,
			encrypted_blob = excluded.encrypted_blob, key_hash = excluded.key_hash,
			status = excluded.status, updated_at = excluded.updated_at`,
		c.ID, c.ProjectID, c.EnvID, c.ConnectorType, c.AccountID, c.Label, c.Description,
		c.SourceType, c.EncryptedBlob, c.KeyHash, c.WrappingKeyID,
		c.SecretRef, c.SecretVersion, c.Status, c.Version,
		c.ExpiresAt, c.RotatedAt, c.RotatedBy, c.CreatedBy, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	return err
}

func (s *SQLiteAppStore) ListCredentials(ctx context.Context, envID string, page store.Page) (store.PageResult[*control.ConnectorCredential], error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, env_id, connector_type, account_id, label, description,
		       source_type, encrypted_blob, key_hash, wrapping_key_id,
		       secret_ref, secret_version, status, version,
		       expires_at, rotated_at, rotated_by, last_used_at, last_validated_at,
		       created_by, created_at, updated_at, revoked_at, revoked_by, revoke_reason
		FROM credentials WHERE env_id = ? AND status = 'active' ORDER BY created_at DESC`,
		envID)
	if err != nil {
		return store.PageResult[*control.ConnectorCredential]{}, err
	}
	defer rows.Close()

	var items []*control.ConnectorCredential
	for rows.Next() {
		c, err := s.scanCredentialRow(rows)
		if err != nil {
			return store.PageResult[*control.ConnectorCredential]{}, err
		}
		items = append(items, c)
	}
	return store.Paginate(items, page), rows.Err()
}

func (s *SQLiteAppStore) ResolveCredential(ctx context.Context, filter *control.CredentialFilter) (*control.ConnectorCredential, error) {
	label := filter.Label
	if label == "" {
		label = "primary"
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, env_id, connector_type, account_id, label, description,
		       source_type, encrypted_blob, key_hash, wrapping_key_id,
		       secret_ref, secret_version, status, version,
		       expires_at, rotated_at, rotated_by, last_used_at, last_validated_at,
		       created_by, created_at, updated_at, revoked_at, revoked_by, revoke_reason
		FROM credentials
		WHERE env_id = ? AND connector_type = ? AND status = 'active'
		ORDER BY CASE WHEN label = ? THEN 0 ELSE 1 END, created_at DESC
		LIMIT 1`, filter.EnvID, filter.ConnectorType, label)
	return s.scanCredential(row)
}

func (s *SQLiteAppStore) RotateCredential(ctx context.Context, id string, newBlob []byte, wrappingKeyID, rotatedBy string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		UPDATE credentials SET
			encrypted_blob = ?, wrapping_key_id = ?, rotated_at = ?, rotated_by = ?,
			version = version + 1, updated_at = ?
		WHERE id = ?`,
		newBlob, wrappingKeyID, now, rotatedBy, now, id)
	return err
}

func (s *SQLiteAppStore) RevokeCredential(ctx context.Context, id, revokedBy, reason string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		UPDATE credentials SET status = 'revoked', revoked_at = ?, revoked_by = ?, revoke_reason = ?, updated_at = ?
		WHERE id = ?`,
		now, revokedBy, reason, now, id)
	return err
}

func (s *SQLiteAppStore) TouchCredential(ctx context.Context, id string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE credentials SET last_used_at = ? WHERE id = ?`, now, id)
	return err
}

func (s *SQLiteAppStore) scanCredential(row *sql.Row) (*control.ConnectorCredential, error) {
	var c control.ConnectorCredential
	err := row.Scan(
		&c.ID, &c.ProjectID, &c.EnvID, &c.ConnectorType, &c.AccountID, &c.Label, &c.Description,
		&c.SourceType, &c.EncryptedBlob, &c.KeyHash, &c.WrappingKeyID,
		&c.SecretRef, &c.SecretVersion, &c.Status, &c.Version,
		&c.ExpiresAt, &c.RotatedAt, &c.RotatedBy, &c.LastUsedAt, &c.LastValidatedAt,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.RevokedAt, &c.RevokedBy, &c.RevokeReason)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &c, err
}

func (s *SQLiteAppStore) scanCredentialRow(rows *sql.Rows) (*control.ConnectorCredential, error) {
	var c control.ConnectorCredential
	if err := rows.Scan(
		&c.ID, &c.ProjectID, &c.EnvID, &c.ConnectorType, &c.AccountID, &c.Label, &c.Description,
		&c.SourceType, &c.EncryptedBlob, &c.KeyHash, &c.WrappingKeyID,
		&c.SecretRef, &c.SecretVersion, &c.Status, &c.Version,
		&c.ExpiresAt, &c.RotatedAt, &c.RotatedBy, &c.LastUsedAt, &c.LastValidatedAt,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.RevokedAt, &c.RevokedBy, &c.RevokeReason); err != nil {
		return nil, err
	}
	return &c, nil
}

// ── Transaction ───────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) WithTx(ctx context.Context, fn func(store.AppStore) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&txAppStore{tx: tx, parent: s}); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// txAppStore wraps a sql.Tx for the subset of AppStore methods needed in atomic seeding.
type txAppStore struct {
	tx     *sql.Tx
	parent *SQLiteAppStore
}

func (t *txAppStore) CreateOrg(ctx context.Context, o *control.Organization) error {
	_, err := t.tx.ExecContext(ctx,
		`INSERT INTO orgs (id, name, slug, plan, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		o.ID, o.Name, o.Slug, o.Plan, o.CreatedAt, o.UpdatedAt)
	return err
}

func (t *txAppStore) CreateProject(ctx context.Context, p *control.Project) error {
	_, err := t.tx.ExecContext(ctx,
		`INSERT INTO projects (id, org_id, name, slug, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.OrgID, p.Name, p.Slug, p.Description, p.CreatedAt, p.UpdatedAt)
	return err
}

func (t *txAppStore) CreateEnvironment(ctx context.Context, e *control.Environment) error {
	_, err := t.tx.ExecContext(ctx,
		`INSERT INTO environments (id, project_id, name, slug, type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.ProjectID, e.Name, e.Slug, e.Type, e.CreatedAt, e.UpdatedAt)
	return err
}

func (t *txAppStore) CreateAgent(ctx context.Context, a *control.Agent) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	var budgetNanos *int64
	if a.MonthlyBudget != nil {
		v := int64(*a.MonthlyBudget)
		budgetNanos = &v
	}
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO agents (id, project_id, env_id, name, description, policy_id, monthly_budget_nanos, status, metadata, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ProjectID, a.EnvID, a.Name, a.Description, a.PolicyID, budgetNanos,
		a.Status, string(metaJSON), a.CreatedBy, a.UpdatedBy, a.CreatedAt, a.UpdatedAt)
	return err
}

func (t *txAppStore) CreatePolicy(ctx context.Context, p *control.PolicyRecord) error {
	typesJSON, _ := json.Marshal(p.AllowedTypes)
	connectorsJSON, _ := json.Marshal(p.AllowedConnectors)
	methodsJSON, _ := json.Marshal(p.AllowedMethods)
	configJSON, _ := json.Marshal(p.Config)
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO policies (
			id, project_id, env_id, name, description,
			allowed_types, allowed_connectors, allowed_methods,
			budget_cap_nanos, budget_period, budget_behavior,
			trace_input, trace_output, retention_days, config,
			version, mode, status, created_by, updated_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ProjectID, p.EnvID, p.Name, p.Description,
		string(typesJSON), string(connectorsJSON), string(methodsJSON),
		int64(p.BudgetCap), p.BudgetPeriod, p.BudgetBehavior,
		boolToInt(p.TraceInput), boolToInt(p.TraceOutput), p.RetentionDays, string(configJSON),
		p.Version, p.Mode, string(p.Status), p.CreatedBy, p.UpdatedBy, p.CreatedAt, p.UpdatedAt)
	return err
}

func (t *txAppStore) CreateRun(ctx context.Context, r *runtimemodel.RunRecord) error {
	metaJSON, _ := json.Marshal(r.Metadata)
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO runs (
			id, project_id, env_id, agent_id, policy_id, name, status,
			budget_cap_nanos, spent_nanos, metadata, error_message,
			trigger_type, trigger_id, correlation_id, session_id, idempotency_key,
			started_at, ended_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.EnvID, r.AgentID, r.PolicyID, r.Name, r.Status,
		int64(r.BudgetCap), int64(r.Spent), string(metaJSON), r.ErrorMessage,
		r.TriggerType, r.TriggerID, r.CorrelationID, r.SessionID, r.IdempotencyKey,
		r.StartedAt, r.EndedAt, r.CreatedAt, r.UpdatedAt)
	return err
}

func (t *txAppStore) InsertBudgetEntry(ctx context.Context, entry *runtimemodel.BudgetEntry) error {
	metaJSON, _ := json.Marshal(entry.Metadata)
	usageJSON, _ := json.Marshal(entry.UsageDetail)
	snapshotJSON, _ := json.Marshal(entry.PriceSnapshot)
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO budget_ledger (
			id, project_id, env_id, policy_id, agent_id, run_id, action_id, span_id,
			connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			reasoning_tokens, audio_input_tokens, audio_output_tokens, image_units,
			request_count, compute_ms, storage_bytes, bandwidth_bytes,
			cost_nanos, price_version, price_snapshot, usage_detail, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.ProjectID, entry.EnvID, entry.PolicyID, entry.AgentID,
		emptyToNil(entry.RunID), entry.ActionID, entry.SpanID,
		entry.Connector, entry.Model,
		entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens,
		entry.ReasoningTokens, entry.AudioInputTokens, entry.AudioOutputTokens, entry.ImageUnits,
		entry.RequestCount, entry.ComputeMs, entry.StorageBytes, entry.BandwidthBytes,
		int64(entry.Cost), emptyToNil(entry.PriceVersion), string(snapshotJSON), string(usageJSON), string(metaJSON), entry.CreatedAt)
	return err
}

func (t *txAppStore) AddRunSpend(ctx context.Context, runID string, cost money.Amount) error {
	_, err := t.tx.ExecContext(ctx, `UPDATE runs SET spent_nanos = spent_nanos + ? WHERE id = ?`, int64(cost), runID)
	return err
}

func (t *txAppStore) StoreCredential(ctx context.Context, c *control.ConnectorCredential) error {
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO credentials (
			id, project_id, env_id, connector_type, account_id, label, description,
			source_type, encrypted_blob, key_hash, wrapping_key_id,
			secret_ref, secret_version, status, version, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProjectID, c.EnvID, c.ConnectorType, c.AccountID, c.Label, c.Description,
		c.SourceType, c.EncryptedBlob, c.KeyHash, c.WrappingKeyID,
		c.SecretRef, c.SecretVersion, c.Status, c.Version, c.CreatedBy, c.CreatedAt, c.UpdatedAt)
	return err
}

func (t *txAppStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := t.tx.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	return err
}

// Delegate read-only methods to parent store.
func (t *txAppStore) GetOrg(ctx context.Context, id string) (*control.Organization, error) {
	return t.parent.GetOrg(ctx, id)
}
func (t *txAppStore) GetOrgBySlug(ctx context.Context, slug string) (*control.Organization, error) {
	return t.parent.GetOrgBySlug(ctx, slug)
}
func (t *txAppStore) ListOrgs(ctx context.Context, page store.Page) (store.PageResult[*control.Organization], error) {
	return t.parent.ListOrgs(ctx, page)
}
func (t *txAppStore) GetProject(ctx context.Context, id string) (*control.Project, error) {
	return t.parent.GetProject(ctx, id)
}
func (t *txAppStore) GetEnvironment(ctx context.Context, id string) (*control.Environment, error) {
	return t.parent.GetEnvironment(ctx, id)
}
func (t *txAppStore) GetEnvironmentBySlug(ctx context.Context, projectID, slug string) (*control.Environment, error) {
	return t.parent.GetEnvironmentBySlug(ctx, projectID, slug)
}
func (t *txAppStore) GetAgentByID(ctx context.Context, id string) (*control.Agent, error) {
	return t.parent.GetAgentByID(ctx, id)
}
func (t *txAppStore) GetAgentByName(ctx context.Context, envID, name string) (*control.Agent, error) {
	return t.parent.GetAgentByName(ctx, envID, name)
}
func (t *txAppStore) GetPolicy(ctx context.Context, id string) (*control.PolicyRecord, error) {
	return t.parent.GetPolicy(ctx, id)
}
func (t *txAppStore) GetAgentPolicy(ctx context.Context, agentID string) (*control.PolicyRecord, error) {
	return t.parent.GetAgentPolicy(ctx, agentID)
}
func (t *txAppStore) DeletePolicy(ctx context.Context, id string) error {
	return t.parent.DeletePolicy(ctx, id)
}
func (t *txAppStore) GetRunByID(ctx context.Context, id string) (*runtimemodel.RunRecord, error) {
	return t.parent.GetRunByID(ctx, id)
}
func (t *txAppStore) GetRunByIdempotencyKey(ctx context.Context, envID, key string) (*runtimemodel.RunRecord, error) {
	return t.parent.GetRunByIdempotencyKey(ctx, envID, key)
}
func (t *txAppStore) GetAction(ctx context.Context, id string) (*runtimemodel.ActionRecord, error) {
	return t.parent.GetAction(ctx, id)
}
func (t *txAppStore) GetCredential(ctx context.Context, id string) (*control.ConnectorCredential, error) {
	return t.parent.GetCredential(ctx, id)
}
func (t *txAppStore) ListFXRates(ctx context.Context) ([]runtimemodel.FXRateRecord, error) {
	return t.parent.ListFXRates(ctx)
}
func (t *txAppStore) GetFXRate(ctx context.Context, base, quote money.CurrencyCode) (*runtimemodel.FXRateRecord, error) {
	return t.parent.GetFXRate(ctx, base, quote)
}
func (t *txAppStore) UpsertFXRates(ctx context.Context, rates []runtimemodel.FXRateRecord) error {
	return t.parent.UpsertFXRates(ctx, rates)
}
func (t *txAppStore) ListFXCurrencies(ctx context.Context) ([]runtimemodel.FXCurrencyRecord, error) {
	return t.parent.ListFXCurrencies(ctx)
}
func (t *txAppStore) UpsertFXCurrencies(ctx context.Context, currencies []runtimemodel.FXCurrencyRecord) error {
	return t.parent.UpsertFXCurrencies(ctx, currencies)
}
func (t *txAppStore) GetPriceBook(ctx context.Context) (*runtimemodel.PriceBook, error) {
	return t.parent.GetPriceBook(ctx)
}
func (t *txAppStore) GetSpendReport(ctx context.Context, filter *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	return t.parent.GetSpendReport(ctx, filter)
}
func (t *txAppStore) SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (money.Amount, error) {
	return t.parent.SumAgentSpend(ctx, agentID, sinceMs)
}
func (t *txAppStore) ResolveCredential(ctx context.Context, filter *control.CredentialFilter) (*control.ConnectorCredential, error) {
	return t.parent.ResolveCredential(ctx, filter)
}

// Stubs for interface completeness (all delegated or no-ops in tx context).
func (t *txAppStore) GetUser(ctx context.Context, id string) (*control.User, error) {
	return t.parent.GetUser(ctx, id)
}
func (t *txAppStore) GetUserByEmail(ctx context.Context, orgID, email string) (*control.User, error) {
	return t.parent.GetUserByEmail(ctx, orgID, email)
}
func (t *txAppStore) GetMembership(ctx context.Context, orgID, userID string) (*control.Membership, error) {
	return t.parent.GetMembership(ctx, orgID, userID)
}

func (t *txAppStore) CreateUser(_ context.Context, _ *control.User) error { return nil }
func (t *txAppStore) UpdateUser(_ context.Context, _ string, _ *control.UserUpdate) error {
	return nil
}
func (t *txAppStore) AddMember(_ context.Context, _ *control.Membership) error { return nil }
func (t *txAppStore) RemoveMember(_ context.Context, _, _ string) error        { return nil }
func (t *txAppStore) ListMembers(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Membership], error) {
	return store.PageResult[*control.Membership]{}, nil
}
func (t *txAppStore) ListProjects(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Project], error) {
	return store.PageResult[*control.Project]{}, nil
}
func (t *txAppStore) ListEnvironments(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Environment], error) {
	return store.PageResult[*control.Environment]{}, nil
}
func (t *txAppStore) UpdateAgent(_ context.Context, _ string, _ *control.AgentUpdate) error {
	return nil
}
func (t *txAppStore) ListAgents(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.Agent], error) {
	return store.PageResult[*control.Agent]{}, nil
}
func (t *txAppStore) DeleteAgent(_ context.Context, _, _ string) error  { return nil }
func (t *txAppStore) RestoreAgent(_ context.Context, _, _ string) error { return nil }
func (t *txAppStore) CreateBudget(ctx context.Context, b *control.Budget) error {
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO budgets (id, agent_id, hard_cap_nanos, soft_cap_nanos, period, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			hard_cap_nanos = excluded.hard_cap_nanos,
			soft_cap_nanos = excluded.soft_cap_nanos,
			period = excluded.period,
			updated_at = excluded.updated_at`,
		b.ID, b.AgentID, int64(b.HardCap), int64(b.SoftCap), b.Period, b.CreatedAt, b.UpdatedAt)
	return err
}
func (t *txAppStore) GetBudget(ctx context.Context, agentID string) (*control.Budget, error) {
	return t.parent.GetBudget(ctx, agentID)
}
func (t *txAppStore) DeleteBudget(ctx context.Context, agentID string) error {
	_, err := t.tx.ExecContext(ctx, `DELETE FROM budgets WHERE agent_id = ?`, agentID)
	return err
}
func (t *txAppStore) ListPolicies(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.PolicyRecord], error) {
	return store.PageResult[*control.PolicyRecord]{}, nil
}
func (t *txAppStore) UpdateRun(_ context.Context, _ string, _ *runtimemodel.RunUpdate) error {
	return nil
}
func (t *txAppStore) ListRuns(_ context.Context, _ *runtimemodel.RunFilter, _ store.Page) (store.PageResult[*runtimemodel.RunRecord], error) {
	return store.PageResult[*runtimemodel.RunRecord]{}, nil
}
func (t *txAppStore) CreateAction(_ context.Context, _ *runtimemodel.ActionRecord) error { return nil }
func (t *txAppStore) ListActionsByRun(_ context.Context, _ string, _ store.Page) (store.PageResult[*runtimemodel.ActionRecord], error) {
	return store.PageResult[*runtimemodel.ActionRecord]{}, nil
}
func (t *txAppStore) SavePriceBook(_ context.Context, _ *runtimemodel.PriceBook) error { return nil }
func (t *txAppStore) UpdatePolicy(ctx context.Context, id string, u *control.PolicyUpdate) error {
	return t.parent.UpdatePolicy(ctx, id, u)
}
func (t *txAppStore) InsertSession(ctx context.Context, session *control.Session) error {
	return t.parent.InsertSession(ctx, session)
}
func (t *txAppStore) GetSessionByHash(ctx context.Context, hash string) (*control.Session, error) {
	return t.parent.GetSessionByHash(ctx, hash)
}
func (t *txAppStore) GetSession(ctx context.Context, id string) (*control.Session, error) {
	return t.parent.GetSession(ctx, id)
}
func (t *txAppStore) ListSessions(ctx context.Context, userID string, page store.Page) (store.PageResult[*control.Session], error) {
	return t.parent.ListSessions(ctx, userID, page)
}
func (t *txAppStore) RevokeSession(ctx context.Context, sessionID, revokedBy string) error {
	return t.parent.RevokeSession(ctx, sessionID, revokedBy)
}
func (t *txAppStore) TouchSession(ctx context.Context, sessionID string) error {
	return t.parent.TouchSession(ctx, sessionID)
}
func (t *txAppStore) InsertAPIToken(ctx context.Context, token *control.APIToken) error {
	return t.parent.InsertAPIToken(ctx, token)
}
func (t *txAppStore) GetAPITokenByHash(ctx context.Context, hash string) (*control.APIToken, error) {
	return t.parent.GetAPITokenByHash(ctx, hash)
}
func (t *txAppStore) GetAPIToken(ctx context.Context, id string) (*control.APIToken, error) {
	return t.parent.GetAPIToken(ctx, id)
}
func (t *txAppStore) ListAPITokens(ctx context.Context, userID string, page store.Page) (store.PageResult[*control.APIToken], error) {
	return t.parent.ListAPITokens(ctx, userID, page)
}
func (t *txAppStore) RevokeAPIToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	return t.parent.RevokeAPIToken(ctx, tokenID, revokedBy, reason)
}
func (t *txAppStore) TouchAPIToken(ctx context.Context, tokenID string) error {
	return t.parent.TouchAPIToken(ctx, tokenID)
}
func (t *txAppStore) GetAgentToken(ctx context.Context, id string) (*control.AgentToken, error) {
	return t.parent.GetAgentToken(ctx, id)
}
func (t *txAppStore) GetAgentTokenByHash(ctx context.Context, hash string) (*control.AgentToken, error) {
	return t.parent.GetAgentTokenByHash(ctx, hash)
}
func (t *txAppStore) ListAgentTokens(ctx context.Context, agentID string, page store.Page) (store.PageResult[*control.AgentToken], error) {
	return t.parent.ListAgentTokens(ctx, agentID, page)
}
func (t *txAppStore) RevokeAgentToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	return t.parent.RevokeAgentToken(ctx, tokenID, revokedBy, reason)
}
func (t *txAppStore) TouchAgentToken(ctx context.Context, tokenID string) error {
	return t.parent.TouchAgentToken(ctx, tokenID)
}
func (t *txAppStore) InsertAgentToken(ctx context.Context, token *control.AgentToken) error {
	return t.parent.InsertAgentToken(ctx, token)
}
func (t *txAppStore) GetTokenByHash(ctx context.Context, hash string) (*control.AgentToken, error) {
	return t.parent.GetTokenByHash(ctx, hash)
}
func (t *txAppStore) GetToken(ctx context.Context, id string) (*control.AgentToken, error) {
	return t.parent.GetToken(ctx, id)
}
func (t *txAppStore) ListTokens(ctx context.Context, agentID string, page store.Page) (store.PageResult[*control.AgentToken], error) {
	return t.parent.ListTokens(ctx, agentID, page)
}
func (t *txAppStore) RevokeToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	return t.parent.RevokeToken(ctx, tokenID, revokedBy, reason)
}
func (t *txAppStore) TouchToken(ctx context.Context, tokenID string) error {
	return t.parent.TouchToken(ctx, tokenID)
}
func (t *txAppStore) ListCredentials(_ context.Context, _ string, _ store.Page) (store.PageResult[*control.ConnectorCredential], error) {
	return store.PageResult[*control.ConnectorCredential]{}, nil
}
func (t *txAppStore) InsertRole(ctx context.Context, role *control.Role) error {
	return t.parent.InsertRole(ctx, role)
}
func (t *txAppStore) GetRole(ctx context.Context, id string) (*control.Role, error) {
	return t.parent.GetRole(ctx, id)
}
func (t *txAppStore) ListRoles(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Role], error) {
	return t.parent.ListRoles(ctx, orgID, page)
}
func (t *txAppStore) UpdateRole(ctx context.Context, id string, role *control.Role) error {
	return t.parent.UpdateRole(ctx, id, role)
}
func (t *txAppStore) DeleteRole(ctx context.Context, id string) error {
	return t.parent.DeleteRole(ctx, id)
}
func (t *txAppStore) InsertBinding(ctx context.Context, binding *control.Binding) error {
	return t.parent.InsertBinding(ctx, binding)
}
func (t *txAppStore) GetBinding(ctx context.Context, id string) (*control.Binding, error) {
	return t.parent.GetBinding(ctx, id)
}
func (t *txAppStore) ListBindings(ctx context.Context, orgID string, page store.Page) (store.PageResult[*control.Binding], error) {
	return t.parent.ListBindings(ctx, orgID, page)
}
func (t *txAppStore) DeleteBinding(ctx context.Context, id string) error {
	return t.parent.DeleteBinding(ctx, id)
}
func (t *txAppStore) RotateCredential(_ context.Context, _ string, _ []byte, _, _ string) error {
	return nil
}
func (t *txAppStore) RevokeCredential(_ context.Context, _, _, _ string) error { return nil }
func (t *txAppStore) TouchCredential(_ context.Context, _ string) error        { return nil }
func (t *txAppStore) WithTx(_ context.Context, _ func(store.AppStore) error) error {
	return fmt.Errorf("cannot nest transactions")
}
func (t *txAppStore) Migrate(_ context.Context) error {
	return fmt.Errorf("cannot migrate in transaction")
}
func (t *txAppStore) Close() error { return fmt.Errorf("cannot close in transaction") }

// ── Helpers ───────────────────────────────────────────────────────────────────

func marshalMetadata(m map[string]any) *string {
	if len(m) == 0 {
		return nil
	}
	b, _ := json.Marshal(m)
	s := string(b)
	return &s
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func pageLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

var _ store.AppStore = (*SQLiteAppStore)(nil)
