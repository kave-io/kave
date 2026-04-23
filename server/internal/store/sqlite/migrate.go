package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all .sql migrations in order from the embedded migrations directory.
// Creates schema_migrations table if it doesn't exist.
// Idempotent: already-applied migrations are skipped.
func Migrate(ctx context.Context, db *sql.DB) error {
	// Set pragmas for this connection
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		return fmt.Errorf("sqlite: set journal_mode: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		return fmt.Errorf("sqlite: set foreign_keys: %w", err)
	}

	// Create schema_migrations table if not exists
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("sqlite: create schema_migrations table: %w", err)
	}

	// Get list of applied migrations
	rows, err := db.QueryContext(ctx, `SELECT name FROM schema_migrations ORDER BY name`)
	if err != nil {
		return fmt.Errorf("sqlite: query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("sqlite: scan migration name: %w", err)
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterate applied migrations: %w", err)
	}

	// List all migration files (.sql files that aren't schema_migrations queries)
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("sqlite: read migrations directory: %w", err)
	}

	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrations = append(migrations, entry.Name())
		}
	}
	sort.Strings(migrations)

	// Run each unapplied migration
	for _, migration := range migrations {
		if applied[migration] {
			continue
		}

		sqlBytes, err := fs.ReadFile(migrationsFS, "migrations/"+migration)
		if err != nil {
			return fmt.Errorf("sqlite: read migration %s: %w", migration, err)
		}

		// Split by semicolon and execute each statement separately
		sqlStr := string(sqlBytes)
		statements := strings.Split(sqlStr, ";")
		for i, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("sqlite: execute migration %s statement %d: %w", migration, i, err)
			}
		}

		// Record migration as applied
		if _, err := db.ExecContext(ctx, `
			INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)
		`, migration, time.Now().UnixMilli()); err != nil {
			return fmt.Errorf("sqlite: record migration %s: %w", migration, err)
		}
	}

	return nil
}
