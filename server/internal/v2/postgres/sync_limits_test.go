package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

func TestSyncLimitsAuditIDBindsScopeAndIdempotencyKey(t *testing.T) {
	t.Parallel()
	first := syncLimitsAuditID("account/a", "namespace/a", "outbox/1")
	if first != syncLimitsAuditID("account/a", "namespace/a", "outbox/1") {
		t.Fatal("audit ID is not deterministic")
	}
	for _, other := range []string{
		syncLimitsAuditID("account/b", "namespace/a", "outbox/1"),
		syncLimitsAuditID("account/a", "namespace/b", "outbox/1"),
		syncLimitsAuditID("account/a", "namespace/a", "outbox/2"),
	} {
		if other == first {
			t.Fatalf("audit ID collision: %q", first)
		}
	}
}

func TestSyncLimitsStorePostgres_GenerationsOwnershipRevisionAndAudit(t *testing.T) {
	dsn := os.Getenv("KAVE_TEST_V2_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_V2_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migrator, err := NewMigrator(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	applyStore, err := NewApplyStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	account := corev2.Ref(ids.New("acc"))
	namespace := corev2.Namespace{
		Account: account, Application: corev2.Ref("sync-" + ids.New("app")), Environment: "test",
	}
	bootstrap := corev2.Caller{
		AccountID: account, ServiceKeyID: "bootstrap",
		Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true,
	}
	created, err := applyStore.Apply(ctx, corev2.ApplyRequest{
		Caller: bootstrap, Manifest: corev2.Manifest{Namespace: namespace}, IdempotencyKey: "bootstrap/limits",
	})
	if err != nil {
		t.Fatalf("bootstrap namespace: %v", err)
	}
	scope := Scope{AccountID: string(account), NamespaceID: string(created.NamespaceID)}
	keyAdmin, err := NewServiceKeyAdmin(pool)
	if err != nil {
		t.Fatal(err)
	}
	keyMaterial := testServiceKeyMaterial(t)
	issued, err := keyAdmin.Issue(ctx, IssueServiceKeyRequest{
		Scope: scope, Name: "subscription-sync", IdempotencyKey: "issue/subscription-sync/1",
		LookupPrefix: keyMaterial.LookupPrefix, SecretHash: keyMaterial.SecretHash[:],
		Operations: []corev2.Operation{corev2.OperationLimitsSync},
	})
	if err != nil {
		t.Fatalf("issue namespace admin key: %v", err)
	}
	caller := corev2.Caller{
		AccountID: account, NamespaceID: created.NamespaceID, ServiceKeyID: corev2.Ref(issued.ID),
		Operations: []corev2.Operation{corev2.OperationLimitsSync},
	}
	runner, err := NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.limits (
    id, account_id, namespace_id, external_key, generation, source, metric,
    hard_cap, window_kind, enabled
) VALUES ($1, $2, $3, 'operator/global', 1, 'operator', 'requests', 999, 'lifetime', TRUE)
`, ids.New("lim"), account, created.NamespaceID)
		return err
	}); err != nil {
		t.Fatalf("seed declarative limit: %v", err)
	}

	store, err := NewLimitSyncStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	soft := int64(8)
	first := corev2.SyncLimitsRequest{
		Caller: caller, NamespaceID: created.NamespaceID, Owner: "simorq/subscriptions", Revision: 1,
		IdempotencyKey: "outbox/1",
		Limits: []corev2.LimitSpec{
			{Key: "clinic/a", Metric: "ai_actions", Selector: corev2.LimitSelector{Tenant: "clinic/a"}, Window: corev2.WindowMonth, HardCap: 10, SoftCap: &soft, Enabled: true},
			{Key: "clinic/b", Metric: "ai_actions", Selector: corev2.LimitSelector{Tenant: "clinic/b"}, Window: corev2.WindowMonth, HardCap: 5, Enabled: true},
		},
	}
	result, err := store.SyncLimits(ctx, first)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if result != (corev2.SyncLimitsResult{Revision: 1, Created: 2}) {
		t.Fatalf("first result = %+v", result)
	}
	var clinicALimitID string
	windowStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		if err := db.QueryRow(ctx, `
SELECT id
FROM kave_v2.limits
WHERE account_id = $1 AND namespace_id = $2
  AND external_key = 'clinic/a' AND superseded_at IS NULL
`, account, created.NamespaceID).Scan(&clinicALimitID); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.limit_windows (
    account_id, namespace_id, limit_id, window_start, window_end, used, reserved
) VALUES ($1, $2, $3, $4, $5, 7, 2)
`, account, created.NamespaceID, clinicALimitID, windowStart, windowStart.AddDate(0, 1, 0))
		return err
	}); err != nil {
		t.Fatalf("seed active synchronized-limit window: %v", err)
	}
	replayed, err := store.SyncLimits(ctx, first)
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if !replayed.Replayed || replayed.Revision != 1 || replayed.Created != 2 {
		t.Fatalf("replayed result = %+v", replayed)
	}

	alias := first
	alias.IdempotencyKey = "outbox/1-alias"
	replayed, err = store.SyncLimits(ctx, alias)
	if err != nil || !replayed.Replayed || replayed.Created != 2 {
		t.Fatalf("same-revision replay = %+v, %v", replayed, err)
	}
	alias.Revision = 2
	if _, err := store.SyncLimits(ctx, alias); !errors.Is(err, corev2.ErrIdempotencyConflict) {
		t.Fatalf("reused idempotency key error = %v", err)
	}

	sameRevisionConflict := first
	sameRevisionConflict.IdempotencyKey = "outbox/1-conflict"
	sameRevisionConflict.Limits = append([]corev2.LimitSpec(nil), first.Limits...)
	sameRevisionConflict.Limits[0].HardCap++
	if _, err := store.SyncLimits(ctx, sameRevisionConflict); !errors.Is(err, corev2.ErrRevisionConflict) {
		t.Fatalf("same-revision conflict error = %v", err)
	}

	second := first
	second.Revision = 2
	second.IdempotencyKey = "outbox/2"
	second.Limits = []corev2.LimitSpec{
		first.Limits[0],
		{Key: "clinic/c", Metric: "ai_actions", Selector: corev2.LimitSelector{Tenant: "clinic/c"}, Window: corev2.WindowDay, HardCap: 2, Enabled: true},
	}
	second.Limits[0].HardCap = 20
	result, err = store.SyncLimits(ctx, second)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if result != (corev2.SyncLimitsResult{Revision: 2, Created: 1, Updated: 1, Disabled: 1}) {
		t.Fatalf("second result = %+v", result)
	}
	if err := assertSyncedLimitPolicyPreservedWindow(ctx, runner, scope, clinicALimitID, windowStart); err != nil {
		t.Fatal(err)
	}

	third := second
	third.Revision = 3
	third.IdempotencyKey = "outbox/3"
	result, err = store.SyncLimits(ctx, third)
	if err != nil {
		t.Fatalf("no-op revision advance: %v", err)
	}
	if result != (corev2.SyncLimitsResult{Revision: 3}) {
		t.Fatalf("no-op result = %+v", result)
	}

	fourth := third
	fourth.Revision = 4
	fourth.IdempotencyKey = "outbox/4"
	fourth.Limits = nil
	result, err = store.SyncLimits(ctx, fourth)
	if err != nil {
		t.Fatalf("empty desired sync: %v", err)
	}
	if result != (corev2.SyncLimitsResult{Revision: 4, Disabled: 2}) {
		t.Fatalf("empty desired result = %+v", result)
	}

	stale := fourth
	stale.Revision = 3
	stale.IdempotencyKey = "outbox/stale"
	if _, err := store.SyncLimits(ctx, stale); !errors.Is(err, corev2.ErrRevisionConflict) {
		t.Fatalf("stale revision error = %v", err)
	}
	ownership := fourth
	ownership.Revision = 5
	ownership.IdempotencyKey = "outbox/ownership"
	ownership.Limits = []corev2.LimitSpec{{Key: "operator/global", Metric: "requests", Window: corev2.WindowAllTime, HardCap: 1, Enabled: true}}
	if _, err := store.SyncLimits(ctx, ownership); !errors.Is(err, corev2.ErrLimitOwnershipConflict) {
		t.Fatalf("ownership conflict error = %v", err)
	}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		_, err := db.Exec(ctx, `
UPDATE kave_v2.limits
SET enabled = FALSE, superseded_at = transaction_timestamp()
WHERE account_id = $1 AND namespace_id = $2
  AND external_key = 'operator/global' AND superseded_at IS NULL
`, account, created.NamespaceID)
		return err
	}); err != nil {
		t.Fatalf("archive operator limit for ownership handoff: %v", err)
	}
	result, err = store.SyncLimits(ctx, ownership)
	if err != nil {
		t.Fatalf("synchronize limit after explicit ownership release: %v", err)
	}
	if result != (corev2.SyncLimitsResult{Revision: 5, Updated: 1}) {
		t.Fatalf("ownership handoff result = %+v", result)
	}

	if err := assertSyncedLimitRows(ctx, runner, scope); err != nil {
		t.Fatal(err)
	}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		var serviceKeyID, requestID, owner, revision string
		if err := db.QueryRow(ctx, `
SELECT service_key_id, request_id, details->>'owner', details->>'source_revision'
FROM kave_v2.audit_events
WHERE account_id = $1 AND namespace_id = $2 AND event = 'limits.sync' AND request_id = 'outbox/2'
`, account, created.NamespaceID).Scan(&serviceKeyID, &requestID, &owner, &revision); err != nil {
			return err
		}
		if serviceKeyID != issued.ID || requestID != "outbox/2" || owner != "simorq/subscriptions" || revision != "2" {
			return fmt.Errorf("audit attribution = %q/%q/%q/%q", serviceKeyID, requestID, owner, revision)
		}
		return nil
	}); err != nil {
		t.Fatalf("verify synchronization audit: %v", err)
	}
}

type syncedLimitRow struct {
	key, source, sourceVersion string
	generation, hardCap        int64
	enabled, superseded        bool
}

func assertSyncedLimitRows(ctx context.Context, runner *ScopedRunner, scope Scope) error {
	return runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		rows, err := db.Query(ctx, `
SELECT external_key, generation, source, source_version, hard_cap, enabled, superseded_at IS NOT NULL
FROM kave_v2.limits
WHERE account_id = $1 AND namespace_id = $2
ORDER BY external_key, generation
`, scope.AccountID, scope.NamespaceID)
		if err != nil {
			return err
		}
		defer rows.Close()
		var got []syncedLimitRow
		for rows.Next() {
			var row syncedLimitRow
			if err := rows.Scan(&row.key, &row.generation, &row.source, &row.sourceVersion,
				&row.hardCap, &row.enabled, &row.superseded); err != nil {
				return err
			}
			got = append(got, row)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		want := []syncedLimitRow{
			{key: "clinic/a", generation: 1, source: "simorq/subscriptions", sourceVersion: "2", hardCap: 20, superseded: true},
			{key: "clinic/b", generation: 1, source: "simorq/subscriptions", sourceVersion: "1", hardCap: 5, superseded: true},
			{key: "clinic/c", generation: 1, source: "simorq/subscriptions", sourceVersion: "2", hardCap: 2, superseded: true},
			{key: "operator/global", generation: 1, source: "operator", sourceVersion: "", hardCap: 999, superseded: true},
			{key: "operator/global", generation: 2, source: "simorq/subscriptions", sourceVersion: "5", hardCap: 1, enabled: true},
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("limit rows = %+v, want %+v", got, want)
		}
		return nil
	})
}

func assertSyncedLimitPolicyPreservedWindow(
	ctx context.Context,
	runner *ScopedRunner,
	scope Scope,
	wantLimitID string,
	windowStart time.Time,
) error {
	return runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		var limitID, sourceVersion string
		var generation, hardCap, revision, used, reserved int64
		if err := db.QueryRow(ctx, `
SELECT l.id, l.generation, l.source_version, l.hard_cap, l.revision, w.used, w.reserved
FROM kave_v2.limits AS l
JOIN kave_v2.limit_windows AS w
  ON w.account_id = l.account_id AND w.namespace_id = l.namespace_id AND w.limit_id = l.id
WHERE l.account_id = $1 AND l.namespace_id = $2
  AND l.external_key = 'clinic/a' AND l.superseded_at IS NULL
  AND w.window_start = $3
`, scope.AccountID, scope.NamespaceID, windowStart).
			Scan(&limitID, &generation, &sourceVersion, &hardCap, &revision, &used, &reserved); err != nil {
			return fmt.Errorf("read synchronized limit after policy update: %w", err)
		}
		if limitID != wantLimitID || generation != 1 || sourceVersion != "2" || hardCap != 20 ||
			revision != 2 || used != 7 || reserved != 2 {
			return fmt.Errorf("synchronized policy update changed counter identity/state: id=%q generation=%d source_version=%q cap/revision=%d/%d used/reserved=%d/%d",
				limitID, generation, sourceVersion, hardCap, revision, used, reserved)
		}
		return nil
	})
}
