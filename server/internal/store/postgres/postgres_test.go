package postgres

import (
	"errors"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestLoadMigrationsSortsAndKeepsSharedNumericPrefixes(t *testing.T) {
	t.Parallel()

	source := fstest.MapFS{
		"migrations/010_trust_mode.up.sql":          {Data: []byte("trust")},
		"migrations/009_trace_fields.up.sql":        {Data: []byte("trace")},
		"migrations/010_budget_block_fields.up.sql": {Data: []byte("budget")},
		"migrations/010_trust_mode.down.sql":        {Data: []byte("ignored")},
		"migrations/MIGRATIONS.md":                  {Data: []byte("ignored")},
	}

	got, err := loadMigrations(source, "migrations")
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}

	wantNames := []string{
		"009_trace_fields.up.sql",
		"010_budget_block_fields.up.sql",
		"010_trust_mode.up.sql",
	}
	gotNames := make([]string, 0, len(got))
	for _, migration := range got {
		gotNames = append(gotNames, migration.name)
		if migration.checksum != migrationChecksum([]byte(migration.sql)) {
			t.Errorf("migration %s checksum does not match its contents", migration.name)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("migration names = %v, want %v", gotNames, wantNames)
	}
}

func TestMigrationChecksumUsesSHA256(t *testing.T) {
	t.Parallel()

	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got := migrationChecksum([]byte("hello")); got != want {
		t.Fatalf("migrationChecksum(hello) = %q, want %q", got, want)
	}
}

func TestBuildMigrationPlanBackfillsLegacyRowsByFullName(t *testing.T) {
	t.Parallel()

	available := []migration{
		{name: "010_budget_block_fields.up.sql", checksum: "budget"},
		{name: "010_trust_mode.up.sql", checksum: "trust"},
		{name: "011_policy_casbin_document.up.sql", checksum: "policy"},
	}
	applied := []appliedMigration{
		{name: "010_budget_block_fields.up.sql"},
		{name: "010_trust_mode.up.sql"},
	}

	plan, err := buildMigrationPlan(available, applied)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}

	if got, want := migrationNames(plan.backfills), []string{
		"010_budget_block_fields.up.sql",
		"010_trust_mode.up.sql",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("backfills = %v, want %v", got, want)
	}
	if got, want := migrationNames(plan.pending), []string{
		"011_policy_casbin_document.up.sql",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pending = %v, want %v", got, want)
	}
}

func TestBuildMigrationPlanPreservesUnknownHistoricalRows(t *testing.T) {
	t.Parallel()

	available := []migration{{name: "011_current.up.sql", checksum: "current"}}
	applied := []appliedMigration{{name: "006_historical.up.sql"}}

	plan, err := buildMigrationPlan(available, applied)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}
	if got, want := migrationNames(plan.pending), []string{"011_current.up.sql"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pending = %v, want %v", got, want)
	}
	if len(plan.backfills) != 0 {
		t.Fatalf("backfills = %v, want none", migrationNames(plan.backfills))
	}
}

func TestBuildMigrationPlanSkipsMatchingChecksum(t *testing.T) {
	t.Parallel()

	available := []migration{{name: "010_trust_mode.up.sql", checksum: "same"}}
	applied := []appliedMigration{{name: "010_trust_mode.up.sql", checksum: "same"}}

	plan, err := buildMigrationPlan(available, applied)
	if err != nil {
		t.Fatalf("buildMigrationPlan: %v", err)
	}
	if len(plan.pending) != 0 || len(plan.backfills) != 0 {
		t.Fatalf(
			"matching migration produced pending=%v backfills=%v",
			migrationNames(plan.pending),
			migrationNames(plan.backfills),
		)
	}
}

func TestBuildMigrationPlanRejectsChecksumDrift(t *testing.T) {
	t.Parallel()

	available := []migration{{name: "010_trust_mode.up.sql", checksum: "embedded"}}
	applied := []appliedMigration{{name: "010_trust_mode.up.sql", checksum: "database"}}

	_, err := buildMigrationPlan(available, applied)
	if err == nil {
		t.Fatal("buildMigrationPlan returned nil error for checksum drift")
	}
	var drift *migrationDriftError
	if !errors.As(err, &drift) {
		t.Fatalf("error type = %T, want *migrationDriftError", err)
	}
	if drift.name != "010_trust_mode.up.sql" {
		t.Fatalf("drift migration = %q, want %q", drift.name, "010_trust_mode.up.sql")
	}
}

func migrationNames(migrations []migration) []string {
	names := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		names = append(names, migration.name)
	}
	return names
}
