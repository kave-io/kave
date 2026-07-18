package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/server/internal/config"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationAdvisoryLockID is the signed 64-bit representation of "KAVEMIGR".
// It is intentionally stable so every Kave process contends on the same
// session-level Postgres advisory lock while changing the schema.
const migrationAdvisoryLockID int64 = 0x4b4156454d494752

const migrationCleanupTimeout = 5 * time.Second

type migration struct {
	name     string
	sql      string
	checksum string
}

type appliedMigration struct {
	name     string
	checksum string
}

type migrationPlan struct {
	pending   []migration
	backfills []migration
}

type migrationDriftError struct {
	name             string
	appliedChecksum  string
	embeddedChecksum string
}

func (e *migrationDriftError) Error() string {
	return fmt.Sprintf(
		"postgres: migration %s checksum mismatch (database=%s embedded=%s)",
		e.name,
		e.appliedChecksum,
		e.embeddedChecksum,
	)
}

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

// Migrate runs all embedded .up.sql migrations in filename order.
//
// A session advisory lock serializes migration runners across Kave processes.
// Each migration and its schema_migrations record are committed atomically.
// Existing records created by the legacy runner receive a checksum the first
// time this runner sees them; subsequent changes to an applied migration fail
// closed as drift. Unknown historical records are preserved for compatibility.
func Migrate(ctx context.Context, pool *pgxpool.Pool) (retErr error) {
	migrations, err := loadMigrations(migrationsFS, "migrations")
	if err != nil {
		return err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("postgres: acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		// A canceled query can have an ambiguous server-side outcome. Discard the
		// session so an acquired lock can never leak back into the pool.
		closeMigrationConnection(conn)
		return fmt.Errorf("postgres: acquire migration advisory lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), migrationCleanupTimeout)
		defer cancel()

		var unlocked bool
		unlockErr := conn.QueryRow(
			unlockCtx,
			`SELECT pg_advisory_unlock($1)`,
			migrationAdvisoryLockID,
		).Scan(&unlocked)
		if unlockErr == nil && unlocked {
			return
		}

		// Never return a pooled session while it may still own an advisory lock.
		// Closing the physical connection makes Postgres release all session locks.
		closeMigrationConnection(conn)
		if retErr == nil {
			if unlockErr != nil {
				retErr = fmt.Errorf("postgres: release migration advisory lock: %w", unlockErr)
			} else {
				retErr = fmt.Errorf("postgres: release migration advisory lock: lock was not held")
			}
		}
	}()

	plan, err := prepareMigrationPlan(ctx, conn, migrations)
	if err != nil {
		return err
	}

	for _, migration := range plan.pending {
		if err := applyMigration(ctx, conn, migration); err != nil {
			return err
		}
	}

	return nil
}

func closeMigrationConnection(conn *pgxpool.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), migrationCleanupTimeout)
	defer cancel()
	_ = conn.Conn().Close(ctx)
}

func rollbackMigrationTx(ctx context.Context, tx pgx.Tx) {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), migrationCleanupTimeout)
	defer cancel()
	_ = tx.Rollback(rollbackCtx)
}

func loadMigrations(source fs.FS, directory string) ([]migration, error) {
	entries, err := fs.ReadDir(source, directory)
	if err != nil {
		return nil, fmt.Errorf("postgres: read migrations directory: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	migrations := make([]migration, 0, len(names))
	for _, name := range names {
		contents, err := fs.ReadFile(source, path.Join(directory, name))
		if err != nil {
			return nil, fmt.Errorf("postgres: read migration %s: %w", name, err)
		}
		migrations = append(migrations, migration{
			name:     name,
			sql:      string(contents),
			checksum: migrationChecksum(contents),
		})
	}

	return migrations, nil
}

func migrationChecksum(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

func buildMigrationPlan(available []migration, applied []appliedMigration) (migrationPlan, error) {
	appliedByName := make(map[string]string, len(applied))
	for _, migration := range applied {
		appliedByName[migration.name] = migration.checksum
	}

	plan := migrationPlan{
		pending:   make([]migration, 0, len(available)),
		backfills: make([]migration, 0, len(available)),
	}
	for _, migration := range available {
		checksum, ok := appliedByName[migration.name]
		if !ok {
			plan.pending = append(plan.pending, migration)
			continue
		}
		if checksum == "" {
			plan.backfills = append(plan.backfills, migration)
			continue
		}
		if checksum != migration.checksum {
			return migrationPlan{}, &migrationDriftError{
				name:             migration.name,
				appliedChecksum:  checksum,
				embeddedChecksum: migration.checksum,
			}
		}
	}

	return plan, nil
}

func prepareMigrationPlan(
	ctx context.Context,
	conn *pgxpool.Conn,
	migrations []migration,
) (migrationPlan, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return migrationPlan{}, fmt.Errorf("postgres: begin migration metadata transaction: %w", err)
	}
	defer rollbackMigrationTx(ctx, tx)

	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			checksum TEXT,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return migrationPlan{}, fmt.Errorf("postgres: create schema_migrations table: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		ALTER TABLE schema_migrations
		ADD COLUMN IF NOT EXISTS checksum TEXT
	`); err != nil {
		return migrationPlan{}, fmt.Errorf("postgres: add migration checksum column: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT name, COALESCE(checksum, '')
		FROM schema_migrations
		ORDER BY name
	`)
	if err != nil {
		return migrationPlan{}, fmt.Errorf("postgres: query applied migrations: %w", err)
	}

	applied := make([]appliedMigration, 0)
	for rows.Next() {
		var migration appliedMigration
		if err := rows.Scan(&migration.name, &migration.checksum); err != nil {
			rows.Close()
			return migrationPlan{}, fmt.Errorf("postgres: scan applied migration: %w", err)
		}
		applied = append(applied, migration)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return migrationPlan{}, fmt.Errorf("postgres: iterate applied migrations: %w", err)
	}

	plan, err := buildMigrationPlan(migrations, applied)
	if err != nil {
		return migrationPlan{}, err
	}
	for _, migration := range plan.backfills {
		tag, err := tx.Exec(ctx, `
			UPDATE schema_migrations
			SET checksum = $2
			WHERE name = $1 AND (checksum IS NULL OR checksum = '')
		`, migration.name, migration.checksum)
		if err != nil {
			return migrationPlan{}, fmt.Errorf("postgres: backfill migration %s checksum: %w", migration.name, err)
		}
		if tag.RowsAffected() != 1 {
			return migrationPlan{}, fmt.Errorf(
				"postgres: backfill migration %s checksum: expected one row, updated %d",
				migration.name,
				tag.RowsAffected(),
			)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return migrationPlan{}, fmt.Errorf("postgres: commit migration metadata transaction: %w", err)
	}
	return plan, nil
}

func applyMigration(ctx context.Context, conn *pgxpool.Conn, migration migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin migration %s: %w", migration.name, err)
	}
	defer rollbackMigrationTx(ctx, tx)

	if _, err := tx.Exec(ctx, migration.sql); err != nil {
		return fmt.Errorf("postgres: execute migration %s: %w", migration.name, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO schema_migrations (name, checksum)
		VALUES ($1, $2)
	`, migration.name, migration.checksum); err != nil {
		return fmt.Errorf("postgres: record migration %s: %w", migration.name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit migration %s: %w", migration.name, err)
	}
	return nil
}
