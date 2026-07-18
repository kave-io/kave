package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockID int64 = 0x4b4156455632 // "KAVEV2"

const ensureMigrationRegistrySQL = `
CREATE SCHEMA IF NOT EXISTS kave_v2;
CREATE TABLE IF NOT EXISTS kave_v2.schema_migrations (
    version     INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    checksum    TEXT NOT NULL CHECK (length(checksum) = 64),
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT transaction_timestamp()
)
`

var migrationNamePattern = regexp.MustCompile(`^(\d{3})_([a-z0-9]+(?:_[a-z0-9]+)*)\.up\.sql$`)

//go:embed migrations/*.up.sql
var embeddedMigrations embed.FS

// Migration is one immutable embedded V2 schema change.
type Migration struct {
	Version  int
	Name     string
	Checksum string
	SQL      string
}

// EmbeddedMigrations returns the validated migrations in version order.
func EmbeddedMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(embeddedMigrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: read embedded migrations: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		match := migrationNamePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("v2 postgres: invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("v2 postgres: parse migration version %q: %w", entry.Name(), err)
		}
		raw, err := fs.ReadFile(embeddedMigrations, path.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("v2 postgres: read migration %q: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(raw)
		migrations = append(migrations, Migration{
			Version:  version,
			Name:     entry.Name(),
			Checksum: hex.EncodeToString(sum[:]),
			SQL:      string(raw),
		})
	}
	if err := validateMigrations(migrations); err != nil {
		return nil, err
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

func validateMigrations(migrations []Migration) error {
	if len(migrations) == 0 {
		return errors.New("v2 postgres: no embedded migrations")
	}
	versions := make(map[int]string, len(migrations))
	names := make(map[string]struct{}, len(migrations))
	for _, migration := range migrations {
		if migration.Version <= 0 {
			return fmt.Errorf("v2 postgres: migration %q has invalid version %d", migration.Name, migration.Version)
		}
		if _, ok := names[migration.Name]; ok {
			return fmt.Errorf("v2 postgres: duplicate migration name %q", migration.Name)
		}
		names[migration.Name] = struct{}{}
		if previous, ok := versions[migration.Version]; ok {
			return fmt.Errorf("v2 postgres: migration version %03d used by %q and %q", migration.Version, previous, migration.Name)
		}
		versions[migration.Version] = migration.Name
		if migration.SQL == "" {
			return fmt.Errorf("v2 postgres: migration %q is empty", migration.Name)
		}
		decodedChecksum, err := hex.DecodeString(migration.Checksum)
		if err != nil || len(decodedChecksum) != sha256.Size {
			return fmt.Errorf("v2 postgres: migration %q has invalid checksum", migration.Name)
		}
	}
	return nil
}

// Migrator owns a migration history entirely separate from the V1 migrator.
type Migrator struct {
	begin beginTxFunc
}

// NewMigrator constructs an embedded V2 migrator.
func NewMigrator(pool *pgxpool.Pool) (*Migrator, error) {
	if pool == nil {
		return nil, ErrNilPool
	}
	return &Migrator{
		begin: func(ctx context.Context, opts pgx.TxOptions) (transaction, error) {
			return pool.BeginTx(ctx, opts)
		},
	}, nil
}

// Migrate applies each migration and its registry row atomically. A
// transaction-scoped advisory lock serializes concurrent server starts.
func (m *Migrator) Migrate(ctx context.Context) error {
	if m == nil || m.begin == nil {
		return ErrNilPool
	}
	migrations, err := EmbeddedMigrations()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if err := m.apply(ctx, migration); err != nil {
			return err
		}
	}
	return nil
}

func (m *Migrator) apply(ctx context.Context, migration Migration) error {
	tx, err := m.begin(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("v2 postgres: begin migration %q: %w", migration.Name, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("v2 postgres: lock migrations: %w", err)
	}
	if _, err := tx.Exec(ctx, ensureMigrationRegistrySQL); err != nil {
		return fmt.Errorf("v2 postgres: ensure migration registry: %w", err)
	}

	var appliedName, appliedChecksum string
	err = tx.QueryRow(ctx, `
SELECT name, checksum
FROM kave_v2.schema_migrations
WHERE version = $1
`, migration.Version).Scan(&appliedName, &appliedChecksum)
	switch {
	case err == nil:
		if appliedName != migration.Name || appliedChecksum != migration.Checksum {
			return fmt.Errorf("v2 postgres: migration %03d drift: database has %q/%s, binary has %q/%s",
				migration.Version, appliedName, appliedChecksum, migration.Name, migration.Checksum)
		}
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			return fmt.Errorf("v2 postgres: execute migration %q: %w", migration.Name, err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO kave_v2.schema_migrations (version, name, checksum)
VALUES ($1, $2, $3)
`, migration.Version, migration.Name, migration.Checksum); err != nil {
			return fmt.Errorf("v2 postgres: record migration %q: %w", migration.Name, err)
		}
	default:
		return fmt.Errorf("v2 postgres: inspect migration %q: %w", migration.Name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("v2 postgres: commit migration %q: %w", migration.Name, err)
	}
	return nil
}
