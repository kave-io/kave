package postgres

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestEmbeddedMigrationsAreStableAndMonotonic(t *testing.T) {
	t.Parallel()

	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v", err)
	}
	if len(migrations) != 1 {
		t.Fatalf("migration count = %d, want 1", len(migrations))
	}
	migration := migrations[0]
	if migration.Version != 1 || migration.Name != "001_kernel.up.sql" {
		t.Fatalf("migration identity = %03d/%q", migration.Version, migration.Name)
	}
	if len(migration.Checksum) != 64 {
		t.Fatalf("checksum length = %d, want 64", len(migration.Checksum))
	}
	if !strings.Contains(migration.SQL, "CREATE SCHEMA IF NOT EXISTS kave_v2") {
		t.Fatal("migration does not create the isolated schema")
	}
}

func TestValidateMigrationsRejectsHistoryDrift(t *testing.T) {
	t.Parallel()

	checksum := strings.Repeat("a", 64)
	tests := []struct {
		name       string
		migrations []Migration
	}{
		{name: "empty"},
		{name: "duplicate version", migrations: []Migration{
			{Version: 1, Name: "001_one.up.sql", Checksum: checksum, SQL: "SELECT 1"},
			{Version: 1, Name: "001_two.up.sql", Checksum: checksum, SQL: "SELECT 2"},
		}},
		{name: "duplicate name", migrations: []Migration{
			{Version: 1, Name: "001_one.up.sql", Checksum: checksum, SQL: "SELECT 1"},
			{Version: 2, Name: "001_one.up.sql", Checksum: checksum, SQL: "SELECT 2"},
		}},
		{name: "empty sql", migrations: []Migration{{Version: 1, Name: "001_one.up.sql", Checksum: checksum}}},
		{name: "bad checksum", migrations: []Migration{{Version: 1, Name: "001_one.up.sql", Checksum: "bad", SQL: "SELECT 1"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateMigrations(tt.migrations); err == nil {
				t.Fatal("validateMigrations() succeeded, want error")
			}
		})
	}
}

func TestKernelMigrationDefinesExactlyTenDomainTables(t *testing.T) {
	t.Parallel()

	sql := kernelMigrationSQL(t)
	re := regexp.MustCompile(`(?m)^CREATE TABLE kave_v2\.([a-z_]+) \(`)
	matches := re.FindAllStringSubmatch(sql, -1)
	got := make([]string, 0, len(matches))
	for _, match := range matches {
		got = append(got, match[1])
	}
	sort.Strings(got)
	want := []string{
		"agents",
		"audit_events",
		"invocations",
		"limit_windows",
		"limits",
		"namespaces",
		"provider_routes",
		"secrets",
		"service_keys",
		"usage_entries",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("domain tables = %v, want %v", got, want)
	}
}

func TestKernelMigrationForcesRLSOnEveryDomainTable(t *testing.T) {
	t.Parallel()

	sql := kernelMigrationSQL(t)
	for _, table := range []string{
		"namespaces", "secrets", "provider_routes", "agents", "service_keys",
		"limits", "limit_windows", "invocations", "usage_entries", "audit_events",
	} {
		if !strings.Contains(sql, "ALTER TABLE kave_v2."+table+" ENABLE ROW LEVEL SECURITY;") {
			t.Errorf("%s does not enable RLS", table)
		}
		if !strings.Contains(sql, "ALTER TABLE kave_v2."+table+" FORCE ROW LEVEL SECURITY;") {
			t.Errorf("%s does not force RLS", table)
		}
	}
	if count := strings.Count(sql, "CREATE POLICY "); count != 11 {
		t.Errorf("policy count = %d, want 11", count)
	}
	if !strings.Contains(sql, "current_setting('kave.account_id', true)") ||
		!strings.Contains(sql, "current_setting('kave.namespace_id', true)") {
		t.Fatal("RLS policies do not use missing-safe account and namespace settings")
	}
}

func TestKernelMigrationScopesBootstrapNamespaceTupleLookup(t *testing.T) {
	t.Parallel()

	sql := kernelMigrationSQL(t)
	start := strings.Index(sql, "CREATE POLICY namespaces_account_isolation")
	end := strings.Index(sql, "ALTER TABLE kave_v2.secrets ENABLE ROW LEVEL SECURITY")
	if start < 0 || end <= start {
		t.Fatal("cannot locate namespace isolation policy")
	}
	policy := sql[start:end]
	for _, required := range []string{
		"id = NULLIF(current_setting('kave.namespace_id', true), '')",
		"application = NULLIF(current_setting('kave.apply_application', true), '')",
		"environment = NULLIF(current_setting('kave.apply_environment', true), '')",
		"WITH CHECK (account_id = NULLIF(current_setting('kave.account_id', true), '')",
	} {
		if !strings.Contains(policy, required) {
			t.Errorf("namespace policy is missing %q", required)
		}
	}
	checkStart := strings.Index(policy, "WITH CHECK")
	if checkStart < 0 {
		t.Fatal("namespace policy has no WITH CHECK clause")
	}
	withCheck := policy[checkStart:]
	if strings.Contains(withCheck, "kave.apply_application") || strings.Contains(withCheck, "kave.apply_environment") {
		t.Fatal("namespace tuple lookup can authorize writes")
	}
}

func TestKernelMigrationEncodesIsolationAndLedgerInvariants(t *testing.T) {
	t.Parallel()

	sql := kernelMigrationSQL(t)
	for _, required := range []string{
		"FOREIGN KEY (account_id, namespace_id)",
		"FOREIGN KEY (account_id, namespace_id, secret_id)",
		"FOREIGN KEY (account_id, namespace_id, route_id)",
		"FOREIGN KEY (account_id, namespace_id, service_key_id)",
		"FOREIGN KEY (account_id, namespace_id, invocation_id)",
		"tenant_ref", "actor_ref", "billing_ref", "session_ref", "feature",
		"UNIQUE (account_id, namespace_id, operation, idempotency_key)",
		"UNIQUE (account_id, namespace_id, external_key, generation)",
		"CREATE UNIQUE INDEX limits_current_key_idx",
		"CREATE UNIQUE INDEX agents_current_name_idx",
		"WHERE status <> 'archived'",
		"CREATE FUNCTION kave_v2.archived_agents_are_immutable()",
		"CREATE TRIGGER archived_agents_are_immutable",
		"WHERE superseded_at IS NULL",
		"CREATE FUNCTION kave_v2.archive_limit_generation_only()",
		"CREATE TRIGGER limits_archive_generation_only",
		"decision            JSONB NOT NULL",
		"octet_length(decision::text) <= 16384",
		"'keys.manage', 'limits.sync', 'usage.read', 'audit.read'",
		"OR cardinality(allowed_agent_ids) > 0",
		"usage_entries_are_immutable", "audit_events_are_immutable",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration is missing %q", required)
		}
	}
	for _, forbidden := range []string{"prompt_body", "response_body", "request_body", "output_body"} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("migration persists forbidden payload field %q", forbidden)
		}
	}
}

func TestKernelMigrationRestrictsPreAuthenticationLookup(t *testing.T) {
	t.Parallel()

	sql := kernelMigrationSQL(t)
	for _, required := range []string{
		"CREATE FUNCTION kave_v2.lookup_service_key(p_lookup_prefix TEXT)",
		"SECURITY DEFINER",
		"SET search_path = pg_catalog, pg_temp",
		"SET row_security = on",
		"REVOKE ALL ON FUNCTION kave_v2.lookup_service_key(TEXT) FROM PUBLIC",
		"current_user <> session_user",
		"lookup_prefix = NULLIF(current_setting('kave.auth_lookup_prefix', true), '')",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("pre-authentication path is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"CREATE OR REPLACE FUNCTION",
		"CREATE TRIGGER IF NOT EXISTS",
	} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("migration relies on rerunnable function/trigger DDL %q", forbidden)
		}
	}

	functionStart := strings.Index(sql, "CREATE FUNCTION kave_v2.lookup_service_key")
	if functionStart < 0 {
		t.Fatal("cannot locate lookup_service_key return shape")
	}
	returnsStart := strings.Index(sql[functionStart:], "RETURNS TABLE (")
	if returnsStart < 0 {
		t.Fatal("cannot locate lookup_service_key RETURNS TABLE")
	}
	returnsEnd := strings.Index(sql[functionStart+returnsStart:], ")\nLANGUAGE")
	if returnsEnd < 0 {
		t.Fatal("cannot locate end of lookup_service_key return shape")
	}
	returns := sql[functionStart+returnsStart : functionStart+returnsStart+returnsEnd]
	for _, required := range []string{
		"account_id", "namespace_id", "service_key_id", "secret_hash",
		"capabilities", "allowed_agent_ids", "can_assert_scope", "status", "expires_at",
	} {
		if !strings.Contains(returns, required) {
			t.Errorf("pre-authentication return shape is missing %q", required)
		}
	}
	for _, forbidden := range []string{"lookup_prefix", "name", "last_used_at", "revoked_at"} {
		if strings.Contains(returns, "\n    "+forbidden+" ") {
			t.Errorf("pre-authentication return shape exposes %q", forbidden)
		}
	}
}

func TestMigratorAppliesSchemaAndRegistryRowInOneTransaction(t *testing.T) {
	t.Parallel()

	tx := &fakeTransaction{}
	tx.queryRow = func(sql string, _ ...any) pgx.Row {
		if !strings.Contains(sql, "kave_v2.schema_migrations") {
			t.Fatalf("unexpected query: %s", sql)
		}
		return scanRow(func(...any) error { return pgx.ErrNoRows })
	}
	migrator := &Migrator{begin: func(context.Context, pgx.TxOptions) (transaction, error) {
		return tx, nil
	}}
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	joined := strings.Join(tx.execs, "\n")
	for _, required := range []string{
		"pg_advisory_xact_lock",
		"CREATE TABLE IF NOT EXISTS kave_v2.schema_migrations",
		"CREATE TABLE kave_v2.namespaces",
		"INSERT INTO kave_v2.schema_migrations",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("migration transaction did not execute %q", required)
		}
	}
	if tx.commits != 1 || tx.rollbacks != 1 {
		t.Fatalf("commits/rollbacks = %d/%d, want 1/1", tx.commits, tx.rollbacks)
	}
}

func TestMigratorRejectsAppliedChecksumDrift(t *testing.T) {
	t.Parallel()

	tx := &fakeTransaction{}
	tx.queryRow = func(_ string, _ ...any) pgx.Row {
		return scanRow(func(dest ...any) error {
			*(dest[0].(*string)) = "001_kernel.up.sql"
			*(dest[1].(*string)) = strings.Repeat("0", 64)
			return nil
		})
	}
	migrator := &Migrator{begin: func(context.Context, pgx.TxOptions) (transaction, error) {
		return tx, nil
	}}
	err := migrator.Migrate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("Migrate() error = %v, want drift error", err)
	}
	if tx.commits != 0 || tx.rollbacks != 1 {
		t.Fatalf("commits/rollbacks = %d/%d, want 0/1", tx.commits, tx.rollbacks)
	}
}

func kernelMigrationSQL(t *testing.T) string {
	t.Helper()
	migrations, err := EmbeddedMigrations()
	if err != nil {
		t.Fatalf("EmbeddedMigrations() error = %v", err)
	}
	return migrations[0].SQL
}
