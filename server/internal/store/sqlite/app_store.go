// Package sqlite provides a SQLite-backed AppStore implementation.
// CGO_ENABLED=1 required — uses mattn/go-sqlite3
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	db *sql.DB
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
	s := &SQLiteAppStore{db: db}
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

// ── OrgStore stubs ────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateOrg(_ context.Context, _ *control.Organization) error { return nil }
func (s *SQLiteAppStore) GetOrg(_ context.Context, _ string) (*control.Organization, error) {
	return nil, nil
}
func (s *SQLiteAppStore) GetOrgBySlug(_ context.Context, _ string) (*control.Organization, error) {
	return nil, nil
}

// ── UserStore stubs ───────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateUser(_ context.Context, _ *control.User) error { return nil }
func (s *SQLiteAppStore) GetUser(_ context.Context, _ string) (*control.User, error) {
	return nil, nil
}
func (s *SQLiteAppStore) GetUserByEmail(_ context.Context, _, _ string) (*control.User, error) {
	return nil, nil
}
func (s *SQLiteAppStore) UpdateUser(_ context.Context, _ string, _ *control.UserUpdate) error {
	return nil
}

// ── MembershipStore stubs ─────────────────────────────────────────────────────

func (s *SQLiteAppStore) AddMember(_ context.Context, _ *control.Membership) error { return nil }
func (s *SQLiteAppStore) GetMembership(_ context.Context, _, _ string) (*control.Membership, error) {
	return nil, nil
}
func (s *SQLiteAppStore) ListMembers(_ context.Context, _ string) ([]*control.Membership, error) {
	return nil, nil
}
func (s *SQLiteAppStore) RemoveMember(_ context.Context, _, _ string) error { return nil }

// ── ProjectStore — maps to workspaces table ───────────────────────────────────

func (s *SQLiteAppStore) CreateProject(ctx context.Context, p *control.Project) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workspaces (id, name, slug, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, p.ID, p.Name, p.Slug, p.Description, p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetProject(ctx context.Context, id string) (*control.Project, error) {
	var p control.Project
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, slug, description, created_at, updated_at FROM workspaces WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Slug, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SQLiteAppStore) ListProjects(_ context.Context, _ string) ([]*control.Project, error) {
	return nil, nil
}

// ── EnvironmentStore stubs ────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateEnvironment(_ context.Context, _ *control.Environment) error {
	return nil
}
func (s *SQLiteAppStore) GetEnvironment(_ context.Context, id string) (*control.Environment, error) {
	if id == "default" {
		return &control.Environment{ID: "default", ProjectID: "default", Name: "Default", Slug: "default"}, nil
	}
	return nil, nil
}
func (s *SQLiteAppStore) GetEnvironmentBySlug(_ context.Context, projectID, slug string) (*control.Environment, error) {
	if slug == "default" {
		return &control.Environment{ID: "default", ProjectID: projectID, Name: "Default", Slug: "default"}, nil
	}
	return nil, nil
}
func (s *SQLiteAppStore) ListEnvironments(_ context.Context, _ string) ([]*control.Environment, error) {
	return []*control.Environment{{ID: "default", ProjectID: "default", Name: "Default", Slug: "default"}}, nil
}

// ── AgentStore ────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateAgent(ctx context.Context, a *control.Agent) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	var budgetDollars *float64
	if a.MonthlyBudget != nil {
		d := a.MonthlyBudget.Dollars()
		budgetDollars = &d
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agents (id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.ProjectID, a.Name, a.Description, a.PolicyID, budgetDollars, string(metaJSON), a.CreatedAt, a.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetAgentByID(ctx context.Context, id string) (*control.Agent, error) {
	return s.getAgent(ctx,
		`SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at FROM agents WHERE id = ?`, id)
}

func (s *SQLiteAppStore) GetAgentByName(ctx context.Context, envID, name string) (*control.Agent, error) {
	return s.getAgent(ctx,
		`SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at FROM agents WHERE workspace_id = ? AND name = ?`, envID, name)
}

func (s *SQLiteAppStore) getAgent(ctx context.Context, query string, args ...any) (*control.Agent, error) {
	var a control.Agent
	var metaJSON string
	var budgetDollars *float64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&a.ID, &a.ProjectID, &a.Name, &a.Description, &a.PolicyID, &budgetDollars, &metaJSON, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if budgetDollars != nil {
		amt := money.FromDollars(*budgetDollars)
		a.MonthlyBudget = &amt
	}
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	if a.Metadata == nil {
		a.Metadata = make(map[string]any)
	}
	a.EnvID = "default"
	a.Status = control.AgentStatusActive
	return &a, nil
}

func (s *SQLiteAppStore) UpdateAgent(ctx context.Context, id string, update *control.AgentUpdate) error {
	now := time.Now().UnixMilli()
	var budgetDollars *float64
	if update.MonthlyBudget != nil {
		d := update.MonthlyBudget.Dollars()
		budgetDollars = &d
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE agents
		SET description = COALESCE(?, description),
		    policy_id = COALESCE(?, policy_id),
		    monthly_budget = COALESCE(?, monthly_budget),
		    metadata = COALESCE(?, metadata),
		    updated_at = ?
		WHERE id = ?
	`, update.Description, update.PolicyID, budgetDollars, marshalMetadata(update.Metadata), now, id)
	return err
}

func (s *SQLiteAppStore) ListAgents(ctx context.Context, envID string) ([]*control.Agent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at
		FROM agents WHERE workspace_id = ? ORDER BY name
	`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*control.Agent
	for rows.Next() {
		var a control.Agent
		var metaJSON string
		var budgetDollars *float64
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Name, &a.Description, &a.PolicyID, &budgetDollars, &metaJSON, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		if budgetDollars != nil {
			amt := money.FromDollars(*budgetDollars)
			a.MonthlyBudget = &amt
		}
		if metaJSON != "" {
			_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
		}
		if a.Metadata == nil {
			a.Metadata = make(map[string]any)
		}
		a.EnvID = "default"
		a.Status = control.AgentStatusActive
		agents = append(agents, &a)
	}
	return agents, rows.Err()
}

// ── PolicyStore ───────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreatePolicy(ctx context.Context, p *control.PolicyRecord) error {
	connectorsJSON, _ := json.Marshal(p.AllowedConnectors)
	methodsJSON, _ := json.Marshal(p.AllowedMethods)
	configJSON, _ := json.Marshal(p.Config)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO policies (id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.ProjectID, p.Name, p.Description, string(connectorsJSON), string(methodsJSON), p.BudgetCap.Dollars(), string(configJSON), p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetPolicy(ctx context.Context, id string) (*control.PolicyRecord, error) {
	var p control.PolicyRecord
	var connectorsJSON, methodsJSON, configJSON string
	var budgetDollars float64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at
		FROM policies WHERE id = ?
	`, id).Scan(&p.ID, &p.ProjectID, &p.Name, &p.Description, &connectorsJSON, &methodsJSON, &budgetDollars, &configJSON, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.BudgetCap = money.FromDollars(budgetDollars)
	p.EnvID = "default"
	p.Mode = control.PolicyModeEnforce
	p.Status = control.PolicyStatusActive
	_ = json.Unmarshal([]byte(connectorsJSON), &p.AllowedConnectors)
	_ = json.Unmarshal([]byte(methodsJSON), &p.AllowedMethods)
	_ = json.Unmarshal([]byte(configJSON), &p.Config)
	return &p, nil
}

func (s *SQLiteAppStore) GetAgentPolicy(ctx context.Context, agentID string) (*control.PolicyRecord, error) {
	var policyID *string
	err := s.db.QueryRowContext(ctx, `SELECT policy_id FROM agents WHERE id = ?`, agentID).Scan(&policyID)
	if err != nil || policyID == nil {
		return nil, err
	}
	return s.GetPolicy(ctx, *policyID)
}

func (s *SQLiteAppStore) ListPolicies(ctx context.Context, envID string) ([]*control.PolicyRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at
		FROM policies WHERE workspace_id = ? ORDER BY name
	`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*control.PolicyRecord
	for rows.Next() {
		var p control.PolicyRecord
		var connectorsJSON, methodsJSON, configJSON string
		var budgetDollars float64
		if err := rows.Scan(&p.ID, &p.ProjectID, &p.Name, &p.Description, &connectorsJSON, &methodsJSON, &budgetDollars, &configJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.BudgetCap = money.FromDollars(budgetDollars)
		p.EnvID = "default"
		p.Mode = control.PolicyModeEnforce
		p.Status = control.PolicyStatusActive
		_ = json.Unmarshal([]byte(connectorsJSON), &p.AllowedConnectors)
		_ = json.Unmarshal([]byte(methodsJSON), &p.AllowedMethods)
		_ = json.Unmarshal([]byte(configJSON), &p.Config)
		policies = append(policies, &p)
	}
	return policies, rows.Err()
}

// ── RunStore ──────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateRun(ctx context.Context, r *runtimemodel.RunRecord) error {
	metaJSON, _ := json.Marshal(r.Metadata)
	var budgetDollars *float64
	if r.BudgetCap != 0 {
		d := r.BudgetCap.Dollars()
		budgetDollars = &d
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runs (id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.ProjectID, r.AgentID, r.PolicyID, r.Name, r.Status, budgetDollars, r.Spent.Dollars(), string(metaJSON), r.ErrorMessage, r.StartedAt, r.EndedAt, r.CreatedAt, r.UpdatedAt)
	return err
}

func (s *SQLiteAppStore) GetRunByID(ctx context.Context, id string) (*runtimemodel.RunRecord, error) {
	var r runtimemodel.RunRecord
	var metaJSON string
	var budgetDollars *float64
	var spentDollars float64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at
		FROM runs WHERE id = ?
	`, id).Scan(&r.ID, &r.ProjectID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &budgetDollars, &spentDollars, &metaJSON, &r.ErrorMessage, &r.StartedAt, &r.EndedAt, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if budgetDollars != nil {
		r.BudgetCap = money.FromDollars(*budgetDollars)
	}
	r.Spent = money.FromDollars(spentDollars)
	r.EnvID = "default"
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &r.Metadata)
	}
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
	return &r, nil
}

func (s *SQLiteAppStore) GetRunByIdempotencyKey(_ context.Context, _, _ string) (*runtimemodel.RunRecord, error) {
	return nil, nil
}

func (s *SQLiteAppStore) UpdateRun(ctx context.Context, id string, update *runtimemodel.RunUpdate) error {
	now := time.Now().UnixMilli()
	var spentDollars *float64
	if update.Spent != nil {
		d := update.Spent.Dollars()
		spentDollars = &d
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE runs
		SET status = COALESCE(?, status),
		    spent_usd = COALESCE(?, spent_usd),
		    error_message = COALESCE(?, error_message),
		    ended_at = COALESCE(?, ended_at),
		    metadata = COALESCE(?, metadata),
		    updated_at = ?
		WHERE id = ?
	`, update.Status, spentDollars, update.ErrorMessage, update.EndedAt, marshalMetadata(update.Metadata), now, id)
	return err
}

func (s *SQLiteAppStore) ListRuns(ctx context.Context, filter *runtimemodel.RunFilter) ([]*runtimemodel.RunRecord, error) {
	query := `
		SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at
		FROM runs WHERE 1=1
	`
	var args []any

	if filter.ProjectID != "" {
		query += ` AND workspace_id = ?`
		args = append(args, filter.ProjectID)
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
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*runtimemodel.RunRecord
	for rows.Next() {
		var r runtimemodel.RunRecord
		var metaJSON string
		var budgetDollars *float64
		var spentDollars float64
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &budgetDollars, &spentDollars, &metaJSON, &r.ErrorMessage, &r.StartedAt, &r.EndedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if budgetDollars != nil {
			r.BudgetCap = money.FromDollars(*budgetDollars)
		}
		r.Spent = money.FromDollars(spentDollars)
		r.EnvID = "default"
		if metaJSON != "" {
			_ = json.Unmarshal([]byte(metaJSON), &r.Metadata)
		}
		if r.Metadata == nil {
			r.Metadata = make(map[string]any)
		}
		runs = append(runs, &r)
	}
	return runs, rows.Err()
}

// ── ActionStore ───────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) CreateAction(ctx context.Context, a *runtimemodel.ActionRecord) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	var inputStr *string
	if a.Input != nil {
		s := string(*a.Input)
		inputStr = &s
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO actions (id, run_id, action_type, connector, method, input, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.RunID, a.ActionType, a.Connector, a.Method, inputStr, string(metaJSON), a.CreatedAt)
	return err
}

func (s *SQLiteAppStore) GetAction(ctx context.Context, id string) (*runtimemodel.ActionRecord, error) {
	var a runtimemodel.ActionRecord
	var metaJSON string
	var inputStr *string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, run_id, action_type, connector, method, input, metadata, created_at
		FROM actions WHERE id = ?
	`, id).Scan(&a.ID, &a.RunID, &a.ActionType, &a.Connector, &a.Method, &inputStr, &metaJSON, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if inputStr != nil {
		b := []byte(*inputStr)
		a.Input = &b
	}
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	a.Source = runtimemodel.ActionSourceIntercepted
	a.Status = runtimemodel.ActionStatusCompleted
	return &a, nil
}

func (s *SQLiteAppStore) ListActionsByRun(ctx context.Context, runID string) ([]*runtimemodel.ActionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, run_id, action_type, connector, method, input, metadata, created_at
		FROM actions WHERE run_id = ? ORDER BY created_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []*runtimemodel.ActionRecord
	for rows.Next() {
		var a runtimemodel.ActionRecord
		var metaJSON string
		var inputStr *string
		if err := rows.Scan(&a.ID, &a.RunID, &a.ActionType, &a.Connector, &a.Method, &inputStr, &metaJSON, &a.CreatedAt); err != nil {
			return nil, err
		}
		if inputStr != nil {
			b := []byte(*inputStr)
			a.Input = &b
		}
		if metaJSON != "" {
			_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
		}
		a.Source = runtimemodel.ActionSourceIntercepted
		a.Status = runtimemodel.ActionStatusCompleted
		actions = append(actions, &a)
	}
	return actions, rows.Err()
}

// ── CostStore ─────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) GetPriceBook(ctx context.Context) (*runtimemodel.PriceBook, error) {
	rows, err := s.db.QueryContext(ctx, `
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
			INSERT INTO price_book_entries (id, version, provider, match, source, input_per_million, output_per_million, cache_read_per_million, cache_write_per_million, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, fmt.Sprintf("%s:%d", book.Version, i), book.Version, entry.Provider, entry.Match, entry.Source,
			entry.InputPerMillion, entry.OutputPerMillion, entry.CacheReadPerMillion, entry.CacheWritePerMillion, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteAppStore) InsertBudgetEntry(ctx context.Context, entry *runtimemodel.BudgetEntry) error {
	metaJSON, _ := json.Marshal(entry.Metadata)
	snapshotJSON, _ := json.Marshal(entry.PriceSnapshot)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO budget_ledger (id, workspace_id, agent_id, run_id, action_id, span_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, price_version, price_snapshot, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.ProjectID, entry.AgentID, emptyToNil(entry.RunID), entry.ActionID, entry.SpanID,
		entry.Connector, entry.Model, entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens,
		entry.Cost.Dollars(), emptyToNil(entry.PriceVersion), string(snapshotJSON), string(metaJSON), entry.CreatedAt)
	return err
}

func (s *SQLiteAppStore) AddRunSpend(ctx context.Context, runID string, cost money.Amount) error {
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET spent_usd = spent_usd + ? WHERE id = ?`, cost.Dollars(), runID)
	return err
}

func (s *SQLiteAppStore) SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (money.Amount, error) {
	var total float64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM budget_ledger WHERE agent_id = ? AND created_at >= ?
	`, agentID, sinceMs).Scan(&total)
	return money.FromDollars(total), err
}

func (s *SQLiteAppStore) GetSpendReport(ctx context.Context, filter *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	where, args := spendWhere(filter)

	var totalDollars float64
	var maxTime, minTime *int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost_usd), 0), MAX(created_at), MIN(created_at) FROM budget_ledger `+where, args...).Scan(&totalDollars, &maxTime, &minTime)
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
		report.PeriodStart = *minTime
	}
	if maxTime != nil {
		report.PeriodEnd = *maxTime
	}

	s.aggregateDim(ctx, report.ByProject, "workspace_id", where, args)
	s.aggregateDim(ctx, report.ByAgent, "agent_id", where, args)
	s.aggregateDim(ctx, report.ByConnector, "connector", where, args)
	s.aggregateDim(ctx, report.ByModel, "COALESCE(model, '')", where, args)

	return report, nil
}

func (s *SQLiteAppStore) aggregateDim(ctx context.Context, dest map[string]money.Amount, dim, where string, args []any) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+dim+`, SUM(cost_usd) FROM budget_ledger `+where+` GROUP BY `+dim, args...)
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

// spendWhere builds the WHERE clause and args from a SpendFilter.
func spendWhere(f *runtimemodel.SpendFilter) (string, []any) {
	where := "WHERE 1=1"
	var args []any
	if f.ProjectID != "" {
		where += " AND workspace_id = ?"
		args = append(args, f.ProjectID)
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

// ── TokenStore ────────────────────────────────────────────────────────────────

func (s *SQLiteAppStore) InsertAgentToken(ctx context.Context, token *control.AgentToken) error {
	connectorsJSON, _ := json.Marshal(token.Connectors)
	methodsJSON, _ := json.Marshal(token.Methods)
	var budgetDollars *float64
	if token.BudgetCap != nil {
		d := token.BudgetCap.Dollars()
		budgetDollars = &d
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_tokens (id, agent_id, connectors, methods, budget_cap_usd, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, token.ID, token.AgentID, string(connectorsJSON), string(methodsJSON), budgetDollars, token.ExpiresAt, token.CreatedAt)
	return err
}

func (s *SQLiteAppStore) GetTokenByHash(_ context.Context, _ string) (*control.AgentToken, error) {
	return nil, nil
}
func (s *SQLiteAppStore) RevokeToken(_ context.Context, _, _, _ string) error { return nil }
func (s *SQLiteAppStore) TouchToken(_ context.Context, _ string) error         { return nil }

func (s *SQLiteAppStore) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM revoked_tokens WHERE token_id = ?`, tokenID).Scan(&count)
	return count > 0, err
}

func (s *SQLiteAppStore) InsertRevokedToken(ctx context.Context, tokenID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO revoked_tokens (id, token_id, revoked_at) VALUES (?, ?, ?)
	`, tokenID+":"+fmt.Sprint(time.Now().UnixMilli()), tokenID, time.Now().UnixMilli())
	return err
}

// ── CredentialStore ───────────────────────────────────────────────────────────

func (s *SQLiteAppStore) GetCredential(ctx context.Context, id string) (*control.ConnectorCredential, error) {
	var c control.ConnectorCredential
	var encrypted []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, connector, label, key_hash, encrypted, last_used_at, created_at
		FROM credentials WHERE id = ?
	`, id).Scan(&c.ID, &c.ProjectID, &c.ConnectorType, &c.Label, &c.KeyHash, &encrypted, &c.LastUsedAt, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.EncryptedBlob = encrypted
	c.SourceType = control.CredSourceEncrypted
	c.Status = control.CredStatusActive
	return &c, nil
}

func (s *SQLiteAppStore) StoreCredential(ctx context.Context, c *control.ConnectorCredential) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO credentials (id, workspace_id, connector, label, key_hash, encrypted, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, connector, key_hash) DO UPDATE SET
			label = excluded.label,
			encrypted = excluded.encrypted
	`, c.ID, c.ProjectID, c.ConnectorType, c.Label, c.KeyHash, c.EncryptedBlob, c.CreatedAt)
	return err
}

func (s *SQLiteAppStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	return err
}

func (s *SQLiteAppStore) ListCredentials(_ context.Context, _ string) ([]*control.ConnectorCredential, error) {
	return nil, nil
}

func (s *SQLiteAppStore) ResolveCredential(ctx context.Context, filter *control.CredentialFilter) (*control.ConnectorCredential, error) {
	// Fallback: find by connector type in the project (using workspace_id as project)
	var c control.ConnectorCredential
	var encrypted []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, connector, label, key_hash, encrypted, last_used_at, created_at
		FROM credentials WHERE workspace_id = ? AND connector = ? ORDER BY created_at DESC LIMIT 1
	`, filter.EnvID, filter.ConnectorType).Scan(&c.ID, &c.ProjectID, &c.ConnectorType, &c.Label, &c.KeyHash, &encrypted, &c.LastUsedAt, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.EncryptedBlob = encrypted
	c.SourceType = control.CredSourceEncrypted
	c.Status = control.CredStatusActive
	return &c, nil
}

func (s *SQLiteAppStore) RotateCredential(_ context.Context, _ string, _ []byte, _, _ string) error {
	return nil
}
func (s *SQLiteAppStore) RevokeCredential(_ context.Context, _, _, _ string) error { return nil }
func (s *SQLiteAppStore) TouchCredential(_ context.Context, _ string) error         { return nil }

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

// txAppStore wraps a transaction; only mutations used in atomic seeding are implemented.
type txAppStore struct {
	tx     *sql.Tx
	parent *SQLiteAppStore
}

func (t *txAppStore) CreateProject(ctx context.Context, p *control.Project) error {
	_, err := t.tx.ExecContext(ctx, `INSERT INTO workspaces (id, name, slug, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Slug, p.Description, p.CreatedAt, p.UpdatedAt)
	return err
}

func (t *txAppStore) CreateAgent(ctx context.Context, a *control.Agent) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	var budgetDollars *float64
	if a.MonthlyBudget != nil {
		d := a.MonthlyBudget.Dollars()
		budgetDollars = &d
	}
	_, err := t.tx.ExecContext(ctx, `INSERT INTO agents (id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ProjectID, a.Name, a.Description, a.PolicyID, budgetDollars, string(metaJSON), a.CreatedAt, a.UpdatedAt)
	return err
}

func (t *txAppStore) CreatePolicy(ctx context.Context, p *control.PolicyRecord) error {
	connectorsJSON, _ := json.Marshal(p.AllowedConnectors)
	methodsJSON, _ := json.Marshal(p.AllowedMethods)
	configJSON, _ := json.Marshal(p.Config)
	_, err := t.tx.ExecContext(ctx, `INSERT INTO policies (id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ProjectID, p.Name, p.Description, string(connectorsJSON), string(methodsJSON), p.BudgetCap.Dollars(), string(configJSON), p.CreatedAt, p.UpdatedAt)
	return err
}

func (t *txAppStore) CreateRun(ctx context.Context, r *runtimemodel.RunRecord) error {
	metaJSON, _ := json.Marshal(r.Metadata)
	_, err := t.tx.ExecContext(ctx, `INSERT INTO runs (id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.AgentID, r.PolicyID, r.Name, r.Status, nil, r.Spent.Dollars(), string(metaJSON), r.ErrorMessage, r.StartedAt, r.EndedAt, r.CreatedAt, r.UpdatedAt)
	return err
}

func (t *txAppStore) InsertBudgetEntry(ctx context.Context, entry *runtimemodel.BudgetEntry) error {
	metaJSON, _ := json.Marshal(entry.Metadata)
	snapshotJSON, _ := json.Marshal(entry.PriceSnapshot)
	_, err := t.tx.ExecContext(ctx, `INSERT INTO budget_ledger (id, workspace_id, agent_id, run_id, action_id, span_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, price_version, price_snapshot, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.ProjectID, entry.AgentID, emptyToNil(entry.RunID), entry.ActionID, entry.SpanID,
		entry.Connector, entry.Model, entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens,
		entry.Cost.Dollars(), emptyToNil(entry.PriceVersion), string(snapshotJSON), string(metaJSON), entry.CreatedAt)
	return err
}

func (t *txAppStore) AddRunSpend(ctx context.Context, runID string, cost money.Amount) error {
	_, err := t.tx.ExecContext(ctx, `UPDATE runs SET spent_usd = spent_usd + ? WHERE id = ?`, cost.Dollars(), runID)
	return err
}

func (t *txAppStore) InsertAgentToken(ctx context.Context, token *control.AgentToken) error {
	connectorsJSON, _ := json.Marshal(token.Connectors)
	methodsJSON, _ := json.Marshal(token.Methods)
	var budgetDollars *float64
	if token.BudgetCap != nil {
		d := token.BudgetCap.Dollars()
		budgetDollars = &d
	}
	_, err := t.tx.ExecContext(ctx, `INSERT INTO agent_tokens (id, agent_id, connectors, methods, budget_cap_usd, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		token.ID, token.AgentID, string(connectorsJSON), string(methodsJSON), budgetDollars, token.ExpiresAt, token.CreatedAt)
	return err
}

func (t *txAppStore) InsertRevokedToken(ctx context.Context, tokenID string) error {
	_, err := t.tx.ExecContext(ctx, `INSERT INTO revoked_tokens (id, token_id, revoked_at) VALUES (?, ?, ?)`,
		tokenID+":"+fmt.Sprint(time.Now().UnixMilli()), tokenID, time.Now().UnixMilli())
	return err
}

func (t *txAppStore) StoreCredential(ctx context.Context, c *control.ConnectorCredential) error {
	_, err := t.tx.ExecContext(ctx, `INSERT INTO credentials (id, workspace_id, connector, label, key_hash, encrypted, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.ProjectID, c.ConnectorType, c.Label, c.KeyHash, c.EncryptedBlob, c.CreatedAt)
	return err
}

func (t *txAppStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := t.tx.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	return err
}

// Delegate read-only methods to parent store via the underlying DB.
func (t *txAppStore) GetProject(ctx context.Context, id string) (*control.Project, error) {
	return t.parent.GetProject(ctx, id)
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
func (t *txAppStore) GetRunByID(ctx context.Context, id string) (*runtimemodel.RunRecord, error) {
	return t.parent.GetRunByID(ctx, id)
}

// Stubs for interface completeness.
func (t *txAppStore) CreateOrg(_ context.Context, _ *control.Organization) error    { return nil }
func (t *txAppStore) GetOrg(_ context.Context, _ string) (*control.Organization, error) {
	return nil, nil
}
func (t *txAppStore) GetOrgBySlug(_ context.Context, _ string) (*control.Organization, error) {
	return nil, nil
}
func (t *txAppStore) CreateUser(_ context.Context, _ *control.User) error      { return nil }
func (t *txAppStore) GetUser(_ context.Context, _ string) (*control.User, error) { return nil, nil }
func (t *txAppStore) GetUserByEmail(_ context.Context, _, _ string) (*control.User, error) {
	return nil, nil
}
func (t *txAppStore) UpdateUser(_ context.Context, _ string, _ *control.UserUpdate) error {
	return nil
}
func (t *txAppStore) AddMember(_ context.Context, _ *control.Membership) error { return nil }
func (t *txAppStore) GetMembership(_ context.Context, _, _ string) (*control.Membership, error) {
	return nil, nil
}
func (t *txAppStore) ListMembers(_ context.Context, _ string) ([]*control.Membership, error) {
	return nil, nil
}
func (t *txAppStore) RemoveMember(_ context.Context, _, _ string) error { return nil }
func (t *txAppStore) ListProjects(_ context.Context, _ string) ([]*control.Project, error) {
	return nil, nil
}
func (t *txAppStore) CreateEnvironment(_ context.Context, _ *control.Environment) error { return nil }
func (t *txAppStore) GetEnvironment(_ context.Context, _ string) (*control.Environment, error) {
	return nil, nil
}
func (t *txAppStore) GetEnvironmentBySlug(_ context.Context, _, _ string) (*control.Environment, error) {
	return nil, nil
}
func (t *txAppStore) ListEnvironments(_ context.Context, _ string) ([]*control.Environment, error) {
	return nil, nil
}
func (t *txAppStore) UpdateAgent(_ context.Context, _ string, _ *control.AgentUpdate) error {
	return nil
}
func (t *txAppStore) ListAgents(_ context.Context, _ string) ([]*control.Agent, error) {
	return nil, nil
}
func (t *txAppStore) GetAgentPolicy(_ context.Context, _ string) (*control.PolicyRecord, error) {
	return nil, nil
}
func (t *txAppStore) ListPolicies(_ context.Context, _ string) ([]*control.PolicyRecord, error) {
	return nil, nil
}
func (t *txAppStore) GetRunByIdempotencyKey(_ context.Context, _, _ string) (*runtimemodel.RunRecord, error) {
	return nil, nil
}
func (t *txAppStore) UpdateRun(_ context.Context, _ string, _ *runtimemodel.RunUpdate) error {
	return nil
}
func (t *txAppStore) ListRuns(_ context.Context, _ *runtimemodel.RunFilter) ([]*runtimemodel.RunRecord, error) {
	return nil, nil
}
func (t *txAppStore) CreateAction(_ context.Context, _ *runtimemodel.ActionRecord) error { return nil }
func (t *txAppStore) GetAction(_ context.Context, _ string) (*runtimemodel.ActionRecord, error) {
	return nil, nil
}
func (t *txAppStore) ListActionsByRun(_ context.Context, _ string) ([]*runtimemodel.ActionRecord, error) {
	return nil, nil
}
func (t *txAppStore) GetPriceBook(_ context.Context) (*runtimemodel.PriceBook, error) {
	return nil, nil
}
func (t *txAppStore) SavePriceBook(_ context.Context, _ *runtimemodel.PriceBook) error { return nil }
func (t *txAppStore) SumAgentSpend(_ context.Context, _ string, _ int64) (money.Amount, error) {
	return 0, nil
}
func (t *txAppStore) GetSpendReport(_ context.Context, _ *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	return nil, nil
}
func (t *txAppStore) GetTokenByHash(_ context.Context, _ string) (*control.AgentToken, error) {
	return nil, nil
}
func (t *txAppStore) RevokeToken(_ context.Context, _, _, _ string) error   { return nil }
func (t *txAppStore) TouchToken(_ context.Context, _ string) error           { return nil }
func (t *txAppStore) IsTokenRevoked(_ context.Context, _ string) (bool, error) { return false, nil }
func (t *txAppStore) GetCredential(_ context.Context, _ string) (*control.ConnectorCredential, error) {
	return nil, nil
}
func (t *txAppStore) ListCredentials(_ context.Context, _ string) ([]*control.ConnectorCredential, error) {
	return nil, nil
}
func (t *txAppStore) ResolveCredential(_ context.Context, _ *control.CredentialFilter) (*control.ConnectorCredential, error) {
	return nil, nil
}
func (t *txAppStore) RotateCredential(_ context.Context, _ string, _ []byte, _, _ string) error {
	return nil
}
func (t *txAppStore) RevokeCredential(_ context.Context, _, _, _ string) error { return nil }
func (t *txAppStore) TouchCredential(_ context.Context, _ string) error         { return nil }
func (t *txAppStore) WithTx(_ context.Context, _ func(store.AppStore) error) error {
	return fmt.Errorf("cannot nest transactions")
}
func (t *txAppStore) Migrate(_ context.Context) error { return fmt.Errorf("cannot migrate in transaction") }
func (t *txAppStore) Close() error                    { return fmt.Errorf("cannot close in transaction") }

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

var _ store.AppStore = (*SQLiteAppStore)(nil)
