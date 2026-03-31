// Package sqlite provides a SQLite-backed AppStore implementation.
// CGO_ENABLED=1 required — uses mattn/go-sqlite3
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/db/sqlite"
)

// SQLiteAppStore implements store.AppStore using SQLite with WAL mode.
type SQLiteAppStore struct {
	db *sql.DB
}

// New creates a new SQLite app store with the given file path.
// Automatically runs migrations on first use.
func New(path string) (*SQLiteAppStore, error) {
	// Open SQLite with WAL mode and foreign key support
	dsn := "file:" + path + "?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open database: %w", err)
	}

	// Set connection pool limits (SQLite serializes writes in WAL mode)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)

	// Verify connection works
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: ping database: %w", err)
	}

	s := &SQLiteAppStore{db: db}

	// Run migrations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *SQLiteAppStore) Close() error {
	return s.db.Close()
}

// Migrate runs pending migrations.
func (s *SQLiteAppStore) Migrate(ctx context.Context) error {
	return sqlite.Migrate(ctx, s.db)
}

// CreateWorkspace creates a new workspace.
func (s *SQLiteAppStore) CreateWorkspace(ctx context.Context, w *store.Workspace) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workspaces (id, name, slug, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, w.ID, w.Name, w.Slug, w.Description, w.CreatedAt, w.UpdatedAt)
	return err
}

// GetWorkspace retrieves a workspace by ID.
func (s *SQLiteAppStore) GetWorkspace(ctx context.Context, id string) (*store.Workspace, error) {
	var w store.Workspace
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, slug, description, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, id).Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &w, nil
}

// CreateAgent creates a new agent.
func (s *SQLiteAppStore) CreateAgent(ctx context.Context, a *store.Agent) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agents (id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.WorkspaceID, a.Name, a.Description, a.PolicyID, a.MonthlyBudget, string(metaJSON), a.CreatedAt, a.UpdatedAt)
	return err
}

// GetAgentByID retrieves an agent by ID.
func (s *SQLiteAppStore) GetAgentByID(ctx context.Context, id string) (*store.Agent, error) {
	return s.getAgent(ctx, `SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at FROM agents WHERE id = ?`, id)
}

// GetAgentByName retrieves an agent by workspace and name.
func (s *SQLiteAppStore) GetAgentByName(ctx context.Context, workspaceID, name string) (*store.Agent, error) {
	return s.getAgent(ctx, `SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at FROM agents WHERE workspace_id = ? AND name = ?`, workspaceID, name)
}

func (s *SQLiteAppStore) getAgent(ctx context.Context, query string, args ...any) (*store.Agent, error) {
	var a store.Agent
	var metaJSON string
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&a.ID, &a.WorkspaceID, &a.Name, &a.Description, &a.PolicyID, &a.MonthlyBudget, &metaJSON, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	if a.Metadata == nil {
		a.Metadata = make(map[string]any)
	}
	return &a, nil
}

// UpdateAgent updates an agent.
func (s *SQLiteAppStore) UpdateAgent(ctx context.Context, id string, update *store.AgentUpdate) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		UPDATE agents
		SET description = COALESCE(?, description),
		    policy_id = COALESCE(?, policy_id),
		    monthly_budget = COALESCE(?, monthly_budget),
		    metadata = COALESCE(?, metadata),
		    updated_at = ?
		WHERE id = ?
	`, update.Description, update.PolicyID, update.MonthlyBudget, marshalMetadata(update.Metadata), now, id)
	return err
}

// ListAgents retrieves all agents in a workspace.
func (s *SQLiteAppStore) ListAgents(ctx context.Context, workspaceID string) ([]*store.Agent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at
		FROM agents WHERE workspace_id = ? ORDER BY name
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*store.Agent
	for rows.Next() {
		var a store.Agent
		var metaJSON string
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Description, &a.PolicyID, &a.MonthlyBudget, &metaJSON, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		if metaJSON != "" {
			_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
		}
		if a.Metadata == nil {
			a.Metadata = make(map[string]any)
		}
		agents = append(agents, &a)
	}
	return agents, rows.Err()
}

// CreatePolicy creates a new policy.
func (s *SQLiteAppStore) CreatePolicy(ctx context.Context, p *store.Policy) error {
	connectorsJSON, _ := json.Marshal(p.AllowedConnectors)
	methodsJSON, _ := json.Marshal(p.AllowedMethods)
	configJSON, _ := json.Marshal(p.Config)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO policies (id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.WorkspaceID, p.Name, p.Description, string(connectorsJSON), string(methodsJSON), p.BudgetCapUSD, string(configJSON), p.CreatedAt, p.UpdatedAt)
	return err
}

// GetPolicy retrieves a policy by ID.
func (s *SQLiteAppStore) GetPolicy(ctx context.Context, id string) (*store.Policy, error) {
	var p store.Policy
	var connectorsJSON, methodsJSON, configJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at
		FROM policies WHERE id = ?
	`, id).Scan(&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &connectorsJSON, &methodsJSON, &p.BudgetCapUSD, &configJSON, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(connectorsJSON), &p.AllowedConnectors)
	_ = json.Unmarshal([]byte(methodsJSON), &p.AllowedMethods)
	_ = json.Unmarshal([]byte(configJSON), &p.Config)
	return &p, nil
}

// GetAgentPolicy retrieves the policy associated with an agent.
func (s *SQLiteAppStore) GetAgentPolicy(ctx context.Context, agentID string) (*store.Policy, error) {
	var policyID *string
	err := s.db.QueryRowContext(ctx, `SELECT policy_id FROM agents WHERE id = ?`, agentID).Scan(&policyID)
	if err != nil || policyID == nil {
		return nil, err
	}
	return s.GetPolicy(ctx, *policyID)
}

// CreateRun creates a new run.
func (s *SQLiteAppStore) CreateRun(ctx context.Context, r *store.Run) error {
	metaJSON, _ := json.Marshal(r.Metadata)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runs (id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.WorkspaceID, r.AgentID, r.PolicyID, r.Name, r.Status, r.BudgetCapUSD, r.SpentUSD, string(metaJSON), r.ErrorMessage, r.StartedAt, r.EndedAt, r.CreatedAt, r.UpdatedAt)
	return err
}

// GetRunByID retrieves a run by ID.
func (s *SQLiteAppStore) GetRunByID(ctx context.Context, id string) (*store.Run, error) {
	var r store.Run
	var metaJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at
		FROM runs WHERE id = ?
	`, id).Scan(&r.ID, &r.WorkspaceID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &r.BudgetCapUSD, &r.SpentUSD, &metaJSON, &r.ErrorMessage, &r.StartedAt, &r.EndedAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if metaJSON != "" {
		_ = json.Unmarshal([]byte(metaJSON), &r.Metadata)
	}
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
	return &r, nil
}

// UpdateRun updates a run.
func (s *SQLiteAppStore) UpdateRun(ctx context.Context, id string, update *store.RunUpdate) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		UPDATE runs
		SET status = COALESCE(?, status),
		    spent_usd = COALESCE(?, spent_usd),
		    error_message = COALESCE(?, error_message),
		    ended_at = COALESCE(?, ended_at),
		    metadata = COALESCE(?, metadata),
		    updated_at = ?
		WHERE id = ?
	`, update.Status, update.SpentUSD, update.ErrorMessage, update.EndedAt, marshalMetadata(update.Metadata), now, id)
	return err
}

// ListRuns retrieves runs matching a filter.
func (s *SQLiteAppStore) ListRuns(ctx context.Context, filter *store.RunFilter) ([]*store.Run, error) {
	query := `
		SELECT id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at
		FROM runs WHERE 1=1
	`
	var args []any

	if filter.WorkspaceID != "" {
		query += ` AND workspace_id = ?`
		args = append(args, filter.WorkspaceID)
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

	var runs []*store.Run
	for rows.Next() {
		var r store.Run
		var metaJSON string
		if err := rows.Scan(&r.ID, &r.WorkspaceID, &r.AgentID, &r.PolicyID, &r.Name, &r.Status, &r.BudgetCapUSD, &r.SpentUSD, &metaJSON, &r.ErrorMessage, &r.StartedAt, &r.EndedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
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

// InsertBudgetEntry inserts a budget ledger entry.
func (s *SQLiteAppStore) InsertBudgetEntry(ctx context.Context, entry *store.BudgetEntry) error {
	metaJSON, _ := json.Marshal(entry.Metadata)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO budget_ledger (id, workspace_id, agent_id, run_id, action_id, span_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.WorkspaceID, entry.AgentID, entry.RunID, entry.ActionID, entry.SpanID, entry.Connector, entry.Model, entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens, entry.CostUSD, string(metaJSON), entry.CreatedAt)
	return err
}

// AddRunSpend updates the spent_usd on a run.
func (s *SQLiteAppStore) AddRunSpend(ctx context.Context, runID string, costUSD float64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE runs SET spent_usd = spent_usd + ? WHERE id = ?
	`, costUSD, runID)
	return err
}

// SumAgentSpend sums the cost for an agent since a given timestamp.
func (s *SQLiteAppStore) SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (float64, error) {
	var total float64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0) FROM budget_ledger WHERE agent_id = ? AND created_at >= ?
	`, agentID, sinceMs).Scan(&total)
	return total, err
}

// GetSpendReport retrieves aggregated spend data.
func (s *SQLiteAppStore) GetSpendReport(ctx context.Context, filter *store.SpendFilter) (*store.SpendReport, error) {
	query := `SELECT COALESCE(SUM(cost_usd), 0), MAX(created_at), MIN(created_at) FROM budget_ledger WHERE 1=1`
	var args []any

	if filter.AgentID != "" {
		query += ` AND agent_id = ?`
		args = append(args, filter.AgentID)
	}
	if filter.Connector != "" {
		query += ` AND connector = ?`
		args = append(args, filter.Connector)
	}
	if filter.Model != "" {
		query += ` AND model = ?`
		args = append(args, filter.Model)
	}
	if filter.FromMs != nil {
		query += ` AND created_at >= ?`
		args = append(args, *filter.FromMs)
	}
	if filter.ToMs != nil {
		query += ` AND created_at <= ?`
		args = append(args, *filter.ToMs)
	}

	var total float64
	var maxTime, minTime *int64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&total, &maxTime, &minTime)
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
		report.PeriodStart = *minTime
	}
	if maxTime != nil {
		report.PeriodEnd = *maxTime
	}

	// Aggregate by agent
	rows, err := s.db.QueryContext(ctx, `SELECT agent_id, SUM(cost_usd) FROM budget_ledger GROUP BY agent_id`)
	if err == nil {
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
	rows, err = s.db.QueryContext(ctx, `SELECT connector, SUM(cost_usd) FROM budget_ledger GROUP BY connector`)
	if err == nil {
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
	rows, err = s.db.QueryContext(ctx, `SELECT COALESCE(model, ''), SUM(cost_usd) FROM budget_ledger GROUP BY model`)
	if err == nil {
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
func (s *SQLiteAppStore) InsertAgentToken(ctx context.Context, token *store.AgentToken) error {
	connectorsJSON, _ := json.Marshal(token.Connectors)
	methodsJSON, _ := json.Marshal(token.Methods)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_tokens (id, agent_id, connectors, methods, budget_cap_usd, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, token.ID, token.AgentID, string(connectorsJSON), string(methodsJSON), token.BudgetCapUSD, token.ExpiresAt, token.CreatedAt)
	return err
}

// IsTokenRevoked checks if a token has been revoked.
func (s *SQLiteAppStore) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM revoked_tokens WHERE token_id = ?`, tokenID).Scan(&count)
	return count > 0, err
}

// InsertRevokedToken marks a token as revoked.
func (s *SQLiteAppStore) InsertRevokedToken(ctx context.Context, tokenID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO revoked_tokens (id, token_id, revoked_at) VALUES (?, ?, ?)
	`, tokenID+":"+fmt.Sprint(time.Now().UnixMilli()), tokenID, time.Now().UnixMilli())
	return err
}

// GetCredential retrieves a credential by workspace and connector.
func (s *SQLiteAppStore) GetCredential(ctx context.Context, workspaceID, connector string) (*store.Credential, error) {
	var c store.Credential
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, connector, label, key_hash, encrypted, last_used_at, created_at
		FROM credentials WHERE workspace_id = ? AND connector = ? ORDER BY created_at DESC LIMIT 1
	`, workspaceID, connector).Scan(&c.ID, &c.WorkspaceID, &c.Connector, &c.Label, &c.KeyHash, &c.Encrypted, &c.LastUsedAt, &c.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// StoreCredential stores a credential (creates or updates).
func (s *SQLiteAppStore) StoreCredential(ctx context.Context, c *store.Credential) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO credentials (id, workspace_id, connector, label, key_hash, encrypted, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id, connector, key_hash) DO UPDATE SET
			label = excluded.label,
			encrypted = excluded.encrypted
	`, c.ID, c.WorkspaceID, c.Connector, c.Label, c.KeyHash, c.Encrypted, c.CreatedAt)
	return err
}

// DeleteCredential deletes a credential by ID.
func (s *SQLiteAppStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	return err
}

// WithTx runs a function within a transaction.
func (s *SQLiteAppStore) WithTx(ctx context.Context, fn func(store.AppStore) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// Create a temporary store that uses the transaction
	txStore := &txAppStore{tx: tx}
	if err := fn(txStore); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// txAppStore wraps a transaction to implement AppStore
type txAppStore struct {
	tx *sql.Tx
}

func (t *txAppStore) CreateWorkspace(ctx context.Context, w *store.Workspace) error {
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO workspaces (id, name, slug, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, w.ID, w.Name, w.Slug, w.Description, w.CreatedAt, w.UpdatedAt)
	return err
}

func (t *txAppStore) GetWorkspace(ctx context.Context, id string) (*store.Workspace, error) {
	var w store.Workspace
	err := t.tx.QueryRowContext(ctx, `
		SELECT id, name, slug, description, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, id).Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &w, nil
}

func (t *txAppStore) CreateAgent(ctx context.Context, a *store.Agent) error {
	metaJSON, _ := json.Marshal(a.Metadata)
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO agents (id, workspace_id, name, description, policy_id, monthly_budget, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.WorkspaceID, a.Name, a.Description, a.PolicyID, a.MonthlyBudget, string(metaJSON), a.CreatedAt, a.UpdatedAt)
	return err
}

func (t *txAppStore) GetAgentByID(ctx context.Context, id string) (*store.Agent, error) {
	// Minimal implementation for transaction
	return nil, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) GetAgentByName(ctx context.Context, workspaceID, name string) (*store.Agent, error) {
	return nil, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) UpdateAgent(ctx context.Context, id string, update *store.AgentUpdate) error {
	return fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) ListAgents(ctx context.Context, workspaceID string) ([]*store.Agent, error) {
	return nil, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) CreatePolicy(ctx context.Context, p *store.Policy) error {
	connectorsJSON, _ := json.Marshal(p.AllowedConnectors)
	methodsJSON, _ := json.Marshal(p.AllowedMethods)
	configJSON, _ := json.Marshal(p.Config)
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO policies (id, workspace_id, name, description, allowed_connectors, allowed_methods, budget_cap_usd, config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.WorkspaceID, p.Name, p.Description, string(connectorsJSON), string(methodsJSON), p.BudgetCapUSD, string(configJSON), p.CreatedAt, p.UpdatedAt)
	return err
}

func (t *txAppStore) GetPolicy(ctx context.Context, id string) (*store.Policy, error) {
	return nil, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) GetAgentPolicy(ctx context.Context, agentID string) (*store.Policy, error) {
	return nil, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) CreateRun(ctx context.Context, r *store.Run) error {
	metaJSON, _ := json.Marshal(r.Metadata)
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO runs (id, workspace_id, agent_id, policy_id, name, status, budget_cap_usd, spent_usd, metadata, error_message, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.WorkspaceID, r.AgentID, r.PolicyID, r.Name, r.Status, r.BudgetCapUSD, r.SpentUSD, string(metaJSON), r.ErrorMessage, r.StartedAt, r.EndedAt, r.CreatedAt, r.UpdatedAt)
	return err
}

func (t *txAppStore) GetRunByID(ctx context.Context, id string) (*store.Run, error) {
	return nil, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) UpdateRun(ctx context.Context, id string, update *store.RunUpdate) error {
	return fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) ListRuns(ctx context.Context, filter *store.RunFilter) ([]*store.Run, error) {
	return nil, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) InsertBudgetEntry(ctx context.Context, entry *store.BudgetEntry) error {
	metaJSON, _ := json.Marshal(entry.Metadata)
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO budget_ledger (id, workspace_id, agent_id, run_id, action_id, span_id, connector, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.WorkspaceID, entry.AgentID, entry.RunID, entry.ActionID, entry.SpanID, entry.Connector, entry.Model, entry.InputTokens, entry.OutputTokens, entry.CacheReadTokens, entry.CacheWriteTokens, entry.CostUSD, string(metaJSON), entry.CreatedAt)
	return err
}

func (t *txAppStore) AddRunSpend(ctx context.Context, runID string, costUSD float64) error {
	_, err := t.tx.ExecContext(ctx, `
		UPDATE runs SET spent_usd = spent_usd + ? WHERE id = ?
	`, costUSD, runID)
	return err
}

func (t *txAppStore) SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (float64, error) {
	return 0, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) GetSpendReport(ctx context.Context, filter *store.SpendFilter) (*store.SpendReport, error) {
	return nil, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) InsertAgentToken(ctx context.Context, token *store.AgentToken) error {
	connectorsJSON, _ := json.Marshal(token.Connectors)
	methodsJSON, _ := json.Marshal(token.Methods)
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO agent_tokens (id, agent_id, connectors, methods, budget_cap_usd, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, token.ID, token.AgentID, string(connectorsJSON), string(methodsJSON), token.BudgetCapUSD, token.ExpiresAt, token.CreatedAt)
	return err
}

func (t *txAppStore) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	return false, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) InsertRevokedToken(ctx context.Context, tokenID string) error {
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO revoked_tokens (id, token_id, revoked_at) VALUES (?, ?, ?)
	`, tokenID+":"+fmt.Sprint(time.Now().UnixMilli()), tokenID, time.Now().UnixMilli())
	return err
}

func (t *txAppStore) GetCredential(ctx context.Context, workspaceID, connector string) (*store.Credential, error) {
	return nil, fmt.Errorf("not implemented in transaction")
}

func (t *txAppStore) StoreCredential(ctx context.Context, c *store.Credential) error {
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO credentials (id, workspace_id, connector, label, key_hash, encrypted, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.WorkspaceID, c.Connector, c.Label, c.KeyHash, c.Encrypted, c.CreatedAt)
	return err
}

func (t *txAppStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := t.tx.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	return err
}

func (t *txAppStore) WithTx(ctx context.Context, fn func(store.AppStore) error) error {
	return fmt.Errorf("cannot nest transactions")
}

func (t *txAppStore) Migrate(ctx context.Context) error {
	return fmt.Errorf("cannot migrate in transaction")
}

func (t *txAppStore) Close() error {
	return fmt.Errorf("cannot close in transaction")
}

// Helper to marshal metadata, using nil if the map is empty
func marshalMetadata(m map[string]any) *string {
	if m == nil || len(m) == 0 {
		return nil
	}
	b, _ := json.Marshal(m)
	s := string(b)
	return &s
}
