package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	auditmodel "github.com/kave-io/kave/core/model/audit"
	"github.com/kave-io/kave/core/store"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteAuditStore implements store.AuditStore using SQLite.
type SQLiteAuditStore struct {
	db *sql.DB
}

// NewAuditStore creates a new SQLite audit store.
func NewAuditStore(db *sql.DB) *SQLiteAuditStore {
	return &SQLiteAuditStore{db: db}
}

// NewAuditStoreFromPath creates a new SQLite audit store from a file path.
func NewAuditStoreFromPath(path string) (*SQLiteAuditStore, error) {
	var dsn string
	if path == ":memory:" || path == "file::memory:?cache=shared" {
		dsn = path
	} else {
		dsn = "file:" + path + "?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"
	}
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
	s := &SQLiteAuditStore{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.migrateAuditOnly(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}
	return s, nil
}

// migrateAuditOnly creates the audit_logs table without running the full migration suite.
func (s *SQLiteAuditStore) migrateAuditOnly(ctx context.Context) error {
	// Set pragmas
	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		return fmt.Errorf("sqlite: set journal_mode: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		return fmt.Errorf("sqlite: set foreign_keys: %w", err)
	}

	// Create audit_logs table if not exists - must be one statement per exec
	statements := []string{
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id            TEXT PRIMARY KEY,
			org_id        TEXT NOT NULL,
			project_id    TEXT,
			env_id        TEXT,
			actor_id      TEXT NOT NULL,
			actor_type    TEXT NOT NULL DEFAULT 'system',
			event         TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id   TEXT NOT NULL,
			diff_before   BLOB,
			diff_after    BLOB,
			ip            TEXT,
			provenance    BLOB,
			created_at    INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_org_id ON audit_logs(org_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_project_id ON audit_logs(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_env_id ON audit_logs(env_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_id ON audit_logs(actor_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_resource_type ON audit_logs(resource_type)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC)`,
	}

	for i, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite: execute statement %d: %w", i, err)
		}
	}

	return nil
}

func (s *SQLiteAuditStore) AppendAudit(ctx context.Context, entry *auditmodel.AuditLog) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_logs (id, org_id, project_id, env_id, actor_id, actor_type, event, resource_type, resource_id, diff_before, diff_after, ip, provenance, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.OrgID, entry.ProjectID, entry.EnvID, entry.ActorID, entry.ActorType,
		entry.Event, entry.ResourceType, entry.ResourceID, entry.DiffBefore, entry.DiffAfter,
		entry.IP, entry.Provenance, entry.CreatedAt)
	return err
}

func (s *SQLiteAuditStore) QueryAudits(ctx context.Context, filter *auditmodel.AuditFilter, page store.Page) (store.PageResult[*auditmodel.AuditLog], error) {
	if filter == nil {
		filter = &auditmodel.AuditFilter{}
	}

	query := `SELECT id, org_id, project_id, env_id, actor_id, actor_type, event, resource_type, resource_id, diff_before, diff_after, ip, provenance, created_at FROM audit_logs WHERE 1=1`
	var args []interface{}

	if filter.OrgID != "" {
		query += ` AND org_id = ?`
		args = append(args, filter.OrgID)
	}
	if filter.ProjectID != "" {
		query += ` AND project_id = ?`
		args = append(args, filter.ProjectID)
	}
	if filter.EnvID != "" {
		query += ` AND env_id = ?`
		args = append(args, filter.EnvID)
	}
	if filter.ActorID != "" {
		query += ` AND actor_id = ?`
		args = append(args, filter.ActorID)
	}
	if filter.ResourceType != "" {
		query += ` AND resource_type = ?`
		args = append(args, filter.ResourceType)
	}
	if filter.ResourceID != "" {
		query += ` AND resource_id = ?`
		args = append(args, filter.ResourceID)
	}
	if filter.Event != "" {
		query += ` AND event = ?`
		args = append(args, filter.Event)
	}
	if filter.FromMs != nil {
		query += ` AND created_at >= ?`
		args = append(args, *filter.FromMs)
	}
	if filter.ToMs != nil {
		query += ` AND created_at <= ?`
		args = append(args, *filter.ToMs)
	}

	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.PageResult[*auditmodel.AuditLog]{}, err
	}
	defer rows.Close()

	var items []*auditmodel.AuditLog
	for rows.Next() {
		var entry auditmodel.AuditLog
		if err := rows.Scan(
			&entry.ID, &entry.OrgID, &entry.ProjectID, &entry.EnvID,
			&entry.ActorID, &entry.ActorType, &entry.Event, &entry.ResourceType,
			&entry.ResourceID, &entry.DiffBefore, &entry.DiffAfter, &entry.IP,
			&entry.Provenance, &entry.CreatedAt); err != nil {
			return store.PageResult[*auditmodel.AuditLog]{}, err
		}
		items = append(items, &entry)
	}

	if err := rows.Err(); err != nil {
		return store.PageResult[*auditmodel.AuditLog]{}, err
	}

	return store.Paginate(items, page), nil
}

func (s *SQLiteAuditStore) Migrate(ctx context.Context) error {
	return Migrate(ctx, s.db)
}

func (s *SQLiteAuditStore) Close() error {
	return s.db.Close()
}
