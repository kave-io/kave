package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// VerifyRuntimeMigrations compares every applied migration name and checksum
// with the binary's immutable embedded manifest. Runtime receives SELECT only
// on this non-tenant registry; it cannot mutate schema history.
func VerifyRuntimeMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return ErrNilPool
	}
	expected, err := EmbeddedMigrations()
	if err != nil {
		return err
	}
	rows, err := pool.Query(ctx, `
SELECT version, name, checksum
FROM kave_v2.schema_migrations
ORDER BY version
`)
	if err != nil {
		return fmt.Errorf("v2 postgres: read runtime migration registry: %w", err)
	}
	defer rows.Close()
	applied := make([]Migration, 0, len(expected))
	for rows.Next() {
		var migration Migration
		if err := rows.Scan(&migration.Version, &migration.Name, &migration.Checksum); err != nil {
			return fmt.Errorf("v2 postgres: scan runtime migration registry: %w", err)
		}
		applied = append(applied, migration)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("v2 postgres: iterate runtime migration registry: %w", err)
	}
	return verifyRuntimeMigrationSet(expected, applied)
}

func verifyRuntimeMigrationSet(expected, applied []Migration) error {
	if len(applied) != len(expected) {
		return fmt.Errorf("v2 postgres: runtime schema migration count is %d, binary requires %d", len(applied), len(expected))
	}
	for i := range expected {
		if applied[i].Version != expected[i].Version || applied[i].Name != expected[i].Name || applied[i].Checksum != expected[i].Checksum {
			return fmt.Errorf("v2 postgres: runtime schema migration %d does not match the binary", expected[i].Version)
		}
	}
	return nil
}

// RuntimeReadiness verifies every database invariant required by a serving
// process. It is intentionally stricter than Ping: a reachable owner login or
// a database at a different migration level must remain not-ready.
func RuntimeReadiness(ctx context.Context, pool *pgxpool.Pool, expectedRole string) error {
	if pool == nil {
		return ErrNilPool
	}
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("v2 postgres: readiness ping: %w", err)
	}
	if err := VerifyRuntimeRole(ctx, pool, expectedRole); err != nil {
		return err
	}
	if err := VerifyRuntimeMigrations(ctx, pool); err != nil {
		return err
	}
	return nil
}
