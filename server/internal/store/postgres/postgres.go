package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/server/internal/config"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func NewPool(ctx context.Context, cfg config.PostgresConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.UnixSocketDSN())
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.Pool.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.Pool.MaxIdleConns)
	poolCfg.MaxConnLifetime = time.Duration(cfg.Pool.ConnMaxLifetimeMinutes) * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	return pool, nil
}

// NewWithDSN creates a pool using an explicit DSN string.
func NewWithDSN(ctx context.Context, dsn string, cfg config.PostgresConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.Pool.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.Pool.MaxIdleConns)
	poolCfg.MaxConnLifetime = time.Duration(cfg.Pool.ConnMaxLifetimeMinutes) * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return pool, nil
}

// Acquire gets a raw connection from the pool.
// Caller must defer conn.Release().
func Acquire(ctx context.Context, pool *pgxpool.Pool) (*pgxpool.Conn, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: acquire connection: %w", err)
	}
	return conn, nil
}

// WithTx runs fn inside a transaction. Rolls back on error, commits on success.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit tx: %w", err)
	}
	return nil
}

// Migrate runs all .up.sql migrations in order from embedded migrations directory.
// Creates schema_migrations table if it doesn't exist.
// Idempotent: already-applied migrations are skipped.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	// Create schema_migrations table if not exists
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("postgres: create schema_migrations table: %w", err)
	}

	// Get list of applied migrations
	rows, err := pool.Query(ctx, `SELECT name FROM schema_migrations ORDER BY name`)
	if err != nil {
		return fmt.Errorf("postgres: query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("postgres: scan migration name: %w", err)
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("postgres: iterate applied migrations: %w", err)
	}

	// List all migration files
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("postgres: read migrations directory: %w", err)
	}

	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			migrations = append(migrations, entry.Name())
		}
	}
	sort.Strings(migrations)

	// Run each unapplied migration
	for _, migration := range migrations {
		if applied[migration] {
			continue
		}

		sql, err := fs.ReadFile(migrationsFS, filepath.Join("migrations", migration))
		if err != nil {
			return fmt.Errorf("postgres: read migration %s: %w", migration, err)
		}

		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("postgres: execute migration %s: %w", migration, err)
		}

		// Record migration as applied
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, migration); err != nil {
			return fmt.Errorf("postgres: record migration %s: %w", migration, err)
		}
	}

	return nil
}
