package duckdb

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
	// Create schema_migrations table if not exists
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       VARCHAR PRIMARY KEY,
			applied_at BIGINT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("duckdb: create schema_migrations table: %w", err)
	}

	// Get list of applied migrations
	rows, err := db.QueryContext(ctx, `SELECT name FROM schema_migrations ORDER BY name`)
	if err != nil {
		return fmt.Errorf("duckdb: query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("duckdb: scan migration name: %w", err)
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("duckdb: iterate applied migrations: %w", err)
	}

	// List all migration files (.sql files)
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("duckdb: read migrations directory: %w", err)
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

		sql, err := fs.ReadFile(migrationsFS, "migrations/"+migration)
		if err != nil {
			return fmt.Errorf("duckdb: read migration %s: %w", migration, err)
		}

		if _, err := db.ExecContext(ctx, string(sql)); err != nil {
			return fmt.Errorf("duckdb: execute migration %s: %w", migration, err)
		}

		// Record migration as applied
		if _, err := db.ExecContext(ctx, `
			INSERT INTO schema_migrations (name, applied_at) VALUES ($1, $2)
		`, migration, time.Now().UnixMilli()); err != nil {
			return fmt.Errorf("duckdb: record migration %s: %w", migration, err)
		}
	}

	return nil
}
