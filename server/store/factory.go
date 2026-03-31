package store

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/config"
	duckdbimpl "github.com/kave-io/kave/server/store/duckdb"
	postgresimpl "github.com/kave-io/kave/server/store/postgres"
	sqliteimpl "github.com/kave-io/kave/server/store/sqlite"
)

// NewAppStore creates an AppStore based on config.
// pool is required for Postgres backend, nil for SQLite.
func NewAppStore(cfg config.StorageConfig, pool *pgxpool.Pool) (store.AppStore, error) {
	switch cfg.Backend {
	case "postgres":
		if pool == nil {
			return nil, fmt.Errorf("postgres backend requires connection pool")
		}
		return postgresimpl.New(pool), nil

	case "sqlite", "":
		path := cfg.SQLitePath
		if path == "" {
			path = "kave.db"
		}
		return sqliteimpl.New(path)

	default:
		return nil, fmt.Errorf("unknown app store backend %q", cfg.Backend)
	}
}

// NewSpanStore creates a SpanStore based on config.
// pool is required for Postgres backend, nil for DuckDB.
func NewSpanStore(cfg config.StorageConfig, pool *pgxpool.Pool) (store.SpanStore, error) {
	switch cfg.SpanBackend {
	case "postgres":
		if pool == nil {
			return nil, fmt.Errorf("postgres backend requires connection pool")
		}
		return postgresimpl.NewSpanStore(pool), nil

	case "duckdb", "":
		path := cfg.DuckDBPath
		if path == "" {
			path = "kave-spans.duckdb"
		}
		return duckdbimpl.New(path)

	default:
		return nil, fmt.Errorf("unknown span store backend %q", cfg.SpanBackend)
	}
}
