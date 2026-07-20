package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

func TestDeterministicNamespaceID(t *testing.T) {
	t.Parallel()
	ns := corev2.Namespace{Account: "account/acme", Application: "simorq", Environment: "prod"}
	first := deterministicNamespaceID(ns)
	second := deterministicNamespaceID(ns)
	if first != second {
		t.Fatalf("namespace IDs differ: %q != %q", first, second)
	}
	if err := first.Validate("namespace_id", true); err != nil {
		t.Fatalf("namespace ID is not a valid ref: %v", err)
	}
	ns.Environment = "stage"
	if got := deterministicNamespaceID(ns); got == first {
		t.Fatalf("different namespace tuple produced the same ID %q", got)
	}
}

func TestSortChangesIsStable(t *testing.T) {
	t.Parallel()
	changes := []corev2.Change{
		{ResourceKind: "limit", Name: "z"},
		{ResourceKind: "agent", Name: "b"},
		{ResourceKind: "namespace", Name: "app"},
		{ResourceKind: "route", Name: "a"},
		{ResourceKind: "agent", Name: "a"},
	}
	sortChanges(changes)
	got := make([]string, len(changes))
	for i, change := range changes {
		got[i] = change.ResourceKind + "/" + string(change.Name)
	}
	want := []string{"namespace/app", "route/a", "agent/a", "agent/b", "limit/z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortChanges() = %v, want %v", got, want)
	}
}

func TestRoutePricingDocumentRoundTripAndDiff(t *testing.T) {
	t.Parallel()
	prices := []corev2.ModelPrice{
		{Model: "gpt-4.1", InputNanosPerMillionTokens: 2, OutputNanosPerMillionTokens: 8,
			CacheReadNanosPerMillionTokens: 1, CacheWriteNanosPerMillionTokens: 3, ReasoningNanosPerMillionTokens: 12},
		{Model: "text-embedding-3-small", InputNanosPerMillionTokens: 1},
	}
	roundTrip := modelPricesFromDocument(routePricingDocument(prices))
	if !reflect.DeepEqual(roundTrip, prices) {
		t.Fatalf("pricing round trip = %+v, want %+v", roundTrip, prices)
	}
	current := routeRow{
		provider: "openai", baseURL: "https://api.openai.com/v1", secretID: "sec",
		allowedModels: []string{"gpt-4.1"}, defaultModel: "gpt-4.1", pricing: prices,
		pricingRevision: 4, status: "active",
	}
	spec := corev2.RouteSpec{
		Provider: "openai", Secret: "secret", AllowedModels: []string{"gpt-4.1"},
		DefaultModel: "gpt-4.1", PricingRevision: 4, Pricing: prices,
	}
	if fields := changedRouteFields(current, spec, current.baseURL, current.secretID, 4); len(fields) != 0 {
		t.Fatalf("unchanged pricing fields = %v", fields)
	}
	spec.Pricing = append([]corev2.ModelPrice(nil), prices...)
	spec.Pricing[0].InputNanosPerMillionTokens++
	if err := validateRoutePricingRevision(current, spec, 4); !errors.Is(err, corev2.ErrRevisionConflict) {
		t.Fatalf("same revision pricing change error = %v, want revision conflict", err)
	}
	if err := validateRoutePricingRevision(current, spec, 5); err != nil {
		t.Fatalf("increased revision pricing change error = %v", err)
	}
	if fields := changedRouteFields(current, spec, current.baseURL, current.secretID, 5); !reflect.DeepEqual(fields, []string{"pricing", "pricing_revision"}) {
		t.Fatalf("changed pricing fields = %v", fields)
	} else if routeChangeRequiresValidation(fields) {
		t.Fatalf("pricing-only fields unexpectedly require live revalidation: %v", fields)
	}
	spec.BaseURL = "https://example.com/v1"
	fields := changedRouteFields(current, spec, spec.BaseURL, current.secretID, 5)
	if !routeChangeRequiresValidation(fields) {
		t.Fatalf("route target change did not require live revalidation: %v", fields)
	}
	current.status = "invalid"
	spec.BaseURL = ""
	spec.Pricing = prices
	if fields := changedRouteFields(current, spec, current.baseURL, current.secretID, 4); len(fields) != 0 {
		t.Fatalf("unchanged pending-validation route fields = %v", fields)
	}
}

func TestApplyStorePostgres_TransactionalIdempotentDryRunAndPrune(t *testing.T) {
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
	store, err := NewApplyStore(pool)
	if err != nil {
		t.Fatal(err)
	}

	account := corev2.Ref(ids.New("acc"))
	ns := corev2.Namespace{Account: account, Application: corev2.Ref("simorq-" + ids.New("app")), Environment: "test"}
	caller := corev2.Caller{
		AccountID: account, ServiceKeyID: "bootstrap",
		Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true,
	}
	bootstrap := corev2.ApplyRequest{
		Caller: caller, Manifest: corev2.Manifest{Namespace: ns}, IdempotencyKey: "bootstrap/1",
	}
	created, err := store.Apply(ctx, bootstrap)
	if err != nil {
		t.Fatalf("bootstrap Apply: %v", err)
	}
	if created.Revision != 1 || !created.Applied || created.NamespaceID == "" || created.Changes[0].Kind != corev2.ChangeCreate {
		t.Fatalf("bootstrap result = %+v", created)
	}

	runner, err := NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{AccountID: string(account), NamespaceID: string(created.NamespaceID)}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		var attributed int
		if err := db.QueryRow(ctx, `
SELECT count(*) FROM kave_v2.audit_events
WHERE account_id = $1 AND namespace_id = $2
  AND event = 'config.apply' AND service_key_id IS NOT NULL
`, account, created.NamespaceID).Scan(&attributed); err != nil {
			return err
		}
		if attributed != 0 {
			return fmt.Errorf("bootstrap audit has a persisted service key")
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.secrets (
    id, account_id, namespace_id, name, backend, ciphertext, wrapping_key_id
) VALUES ($1, $2, $3, 'openai-key', 'encrypted', $4, 'test-key')
`, ids.New("sec"), account, created.NamespaceID, []byte("test-ciphertext")); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.secrets (
    id, account_id, namespace_id, name, backend, external_ref
) VALUES ($1, $2, $3, 'external-key', 'external', 'env://OPENAI_API_KEY')
`, ids.New("sec"), account, created.NamespaceID)
		return err
	}); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	soft := int64(80)
	manifest := corev2.Manifest{
		Namespace: ns,
		Routes: []corev2.RouteSpec{{
			Name: "openai", Provider: "openai", Secret: "openai-key",
			AllowedModels: []string{"gpt-4.1"}, DefaultModel: "gpt-4.1", PricingRevision: 1,
			Pricing: []corev2.ModelPrice{{Model: "gpt-4.1"}},
		}},
		Agents: []corev2.AgentSpec{{Name: "assistant", Kind: corev2.AgentLLM, Route: "openai", Enabled: true}},
		Limits: []corev2.LimitSpec{{Key: "monthly-actions", Metric: "ai_actions", Selector: corev2.LimitSelector{Agent: "assistant"}, Window: corev2.WindowMonth, HardCap: 100, SoftCap: &soft, Enabled: true}},
	}
	externalManifest := manifest
	externalManifest.Routes = append([]corev2.RouteSpec(nil), manifest.Routes...)
	externalManifest.Routes[0].Secret = "external-key"
	_, err = store.Apply(ctx, corev2.ApplyRequest{
		Caller: caller, Manifest: externalManifest, ExpectedRevision: 1, IdempotencyKey: "deploy/external-secret",
	})
	if !errors.Is(err, corev2.ErrInvalidArgument) {
		t.Fatalf("external-backed provider route error = %v, want invalid argument", err)
	}
	apply := corev2.ApplyRequest{
		Caller: caller, Manifest: manifest, ExpectedRevision: 1, IdempotencyKey: "deploy/2",
	}
	applied, err := store.Apply(ctx, apply)
	if err != nil {
		t.Fatalf("config Apply: %v", err)
	}
	if applied.Revision != 2 || !applied.Applied {
		t.Fatalf("config result = %+v", applied)
	}
	replayed, err := store.Apply(ctx, apply)
	if err != nil {
		t.Fatalf("replay Apply: %v", err)
	}
	if !reflect.DeepEqual(replayed, applied) {
		t.Fatalf("replay = %+v, want %+v", replayed, applied)
	}

	conflict := apply
	conflict.Manifest.Limits = append([]corev2.LimitSpec(nil), manifest.Limits...)
	conflict.Manifest.Limits[0].HardCap = 101
	_, err = store.Apply(ctx, conflict)
	if !errors.Is(err, corev2.ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict error = %v", err)
	}
	stale := apply
	stale.IdempotencyKey = "stale/3"
	stale.ExpectedRevision = 1
	_, err = store.Apply(ctx, stale)
	if !errors.Is(err, corev2.ErrRevisionConflict) {
		t.Fatalf("revision conflict error = %v", err)
	}

	// Omission is non-destructive unless prune is explicitly set.
	keep := corev2.ApplyRequest{
		Caller: caller, Manifest: corev2.Manifest{Namespace: ns},
		ExpectedRevision: 2, IdempotencyKey: "keep/3",
	}
	kept, err := store.Apply(ctx, keep)
	if err != nil {
		t.Fatalf("non-pruning Apply: %v", err)
	}
	if kept.Revision != 2 || countChanges(kept.Changes, corev2.ChangeDelete) != 0 {
		t.Fatalf("non-pruning result = %+v", kept)
	}
	assertApplyStatuses(t, ctx, runner, scope, true, false, "active", "invalid")

	// Explicitly disabled resources are still current desired state. Prune must
	// archive them when they are later omitted, rather than retaining their
	// names and limit ownership indefinitely.
	disabledManifest := manifest
	disabledManifest.Agents = append([]corev2.AgentSpec(nil), manifest.Agents...)
	disabledManifest.Limits = append([]corev2.LimitSpec(nil), manifest.Limits...)
	disabledManifest.Agents[0].Enabled = false
	disabledManifest.Limits[0].Enabled = false
	disabled, err := store.Apply(ctx, corev2.ApplyRequest{
		Caller: caller, Manifest: disabledManifest,
		ExpectedRevision: 2, IdempotencyKey: "disable/3",
	})
	if err != nil {
		t.Fatalf("disable Apply: %v", err)
	}
	if disabled.Revision != 3 {
		t.Fatalf("disable result = %+v", disabled)
	}
	assertApplyStatuses(t, ctx, runner, scope, false, false, "disabled", "invalid")

	prune := corev2.ApplyRequest{
		Caller: caller, Manifest: corev2.Manifest{Namespace: ns}, DryRun: true,
		Prune: true, ExpectedRevision: 3, IdempotencyKey: "prune/4",
	}
	preview, err := store.Apply(ctx, prune)
	if err != nil {
		t.Fatalf("prune dry-run: %v", err)
	}
	if preview.Applied || preview.Revision != 4 || countChanges(preview.Changes, corev2.ChangeDelete) != 3 {
		t.Fatalf("prune preview = %+v", preview)
	}
	assertApplyStatuses(t, ctx, runner, scope, false, false, "disabled", "invalid")

	prune.DryRun = false
	pruned, err := store.Apply(ctx, prune)
	if err != nil {
		t.Fatalf("prune Apply: %v", err)
	}
	if !pruned.Applied || pruned.Revision != 4 {
		t.Fatalf("prune result = %+v", pruned)
	}
	assertApplyStatuses(t, ctx, runner, scope, false, true, "archived", "archived")
}

func TestApplyStorePostgres_LimitPolicyChangePreservesActiveCounters(t *testing.T) {
	dsn := os.Getenv("KAVE_TEST_V2_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_V2_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migrator, _ := NewMigrator(pool)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store, _ := NewApplyStore(pool)
	account := corev2.Ref(ids.New("acc"))
	ns := corev2.Namespace{Account: account, Application: corev2.Ref("generation-" + ids.New("app")), Environment: "test"}
	caller := corev2.Caller{AccountID: account, ServiceKeyID: "bootstrap", Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true}
	created, err := store.Apply(ctx, corev2.ApplyRequest{
		Caller: caller, Manifest: corev2.Manifest{Namespace: ns}, IdempotencyKey: "generation/bootstrap",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, _ := NewScopedRunner(pool)
	scope := Scope{AccountID: string(account), NamespaceID: string(created.NamespaceID)}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.secrets (
    id, account_id, namespace_id, name, backend, ciphertext, wrapping_key_id
) VALUES ($1, $2, $3, 'provider', 'encrypted', decode('00', 'hex'), 'test-key')
`, ids.New("sec"), account, created.NamespaceID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	manifest := corev2.Manifest{
		Namespace: ns,
		Routes: []corev2.RouteSpec{{
			Name: "route", Provider: "openai", Secret: "provider",
			AllowedModels: []string{"test-model"}, DefaultModel: "test-model", PricingRevision: 1,
			Pricing: []corev2.ModelPrice{{Model: "test-model"}},
		}},
		Agents: []corev2.AgentSpec{{Name: "assistant", Kind: corev2.AgentLLM, Route: "route", Enabled: true}},
		Limits: []corev2.LimitSpec{{Key: "actions", Metric: "ai_actions", Selector: corev2.LimitSelector{Agent: "assistant"}, Window: corev2.WindowMonth, HardCap: 5, Enabled: true}},
	}
	configured, err := store.Apply(ctx, corev2.ApplyRequest{
		Caller: caller, Manifest: manifest, ExpectedRevision: 1, IdempotencyKey: "generation/one",
	})
	if err != nil {
		t.Fatal(err)
	}
	var oldID string
	windowStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		if err := db.QueryRow(ctx, `
SELECT id FROM kave_v2.limits
WHERE account_id = $1 AND namespace_id = $2
  AND external_key = 'actions' AND superseded_at IS NULL
`, account, created.NamespaceID).Scan(&oldID); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.limit_windows (
    account_id, namespace_id, limit_id, window_start, window_end, used, reserved
) VALUES ($1, $2, $3, $4, $5, 4, 1)
`, account, created.NamespaceID, oldID, windowStart, windowStart.AddDate(0, 1, 0))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	manifest.Limits[0].HardCap = 10
	updated, err := store.Apply(ctx, corev2.ApplyRequest{
		Caller: caller, Manifest: manifest, ExpectedRevision: configured.Revision, IdempotencyKey: "generation/v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != configured.Revision+1 {
		t.Fatalf("updated revision = %d", updated.Revision)
	}
	var newID string
	var generations, archived, activeWindows, used, reserved, hardCap, limitRevision int64
	err = runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		if err := db.QueryRow(ctx, `
SELECT id FROM kave_v2.limits
WHERE account_id = $1 AND namespace_id = $2
  AND external_key = 'actions' AND superseded_at IS NULL
`, account, created.NamespaceID).Scan(&newID); err != nil {
			return err
		}
		if err := db.QueryRow(ctx, `
SELECT
    count(*),
    count(*) FILTER (WHERE superseded_at IS NOT NULL),
	(SELECT count(*) FROM kave_v2.limit_windows WHERE limit_id = $3),
	(SELECT used FROM kave_v2.limit_windows WHERE limit_id = $3),
	(SELECT reserved FROM kave_v2.limit_windows WHERE limit_id = $3),
	max(hard_cap),
	max(revision)
FROM kave_v2.limits
WHERE account_id = $1 AND namespace_id = $2 AND external_key = 'actions'
`, account, created.NamespaceID, oldID).
			Scan(&generations, &archived, &activeWindows, &used, &reserved, &hardCap, &limitRevision); err != nil {
			return err
		}
		// Direct policy writes that bypass reconciliation must not silently
		// avoid the required revision increment.
		_, err := db.Exec(ctx, `UPDATE kave_v2.limits SET hard_cap = 11 WHERE id = $1`, newID)
		return err
	})
	var immutableErr *pgconn.PgError
	if !errors.As(err, &immutableErr) || immutableErr.Code != "55000" {
		t.Fatalf("mutable limit definition error = %v, want SQLSTATE 55000", err)
	}
	if newID != oldID || generations != 1 || archived != 0 || activeWindows != 1 ||
		used != 4 || reserved != 1 || hardCap != 10 || limitRevision != 2 {
		t.Fatalf("old/new=%s/%s generations=%d archived=%d windows=%d used/reserved=%d/%d cap/revision=%d/%d",
			oldID, newID, generations, archived, activeWindows, used, reserved, hardCap, limitRevision)
	}
}

func TestApplyStorePostgres_ConcurrentNamespaceResolution(t *testing.T) {
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
		t.Fatal(err)
	}
	store, err := NewApplyStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	account := corev2.Ref(ids.New("acc"))
	ns := corev2.Namespace{Account: account, Application: corev2.Ref("concurrent-" + ids.New("app")), Environment: "test"}
	caller := corev2.Caller{
		AccountID: account, ServiceKeyID: "bootstrap",
		Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true,
	}

	results := make(chan corev2.ApplyResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.Apply(ctx, corev2.ApplyRequest{
				Caller: caller, Manifest: corev2.Manifest{Namespace: ns},
				IdempotencyKey: corev2.Ref(fmt.Sprintf("concurrent/%d", i)),
			})
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Apply: %v", err)
		}
	}
	var namespaceID corev2.Ref
	for result := range results {
		if result.Revision != 1 || result.NamespaceID == "" {
			t.Fatalf("concurrent result = %+v", result)
		}
		if namespaceID != "" && namespaceID != result.NamespaceID {
			t.Fatalf("namespace IDs differ: %q != %q", namespaceID, result.NamespaceID)
		}
		namespaceID = result.NamespaceID
	}
}

func TestApplyStorePostgres_ResolvesExistingNonDeterministicNamespace(t *testing.T) {
	dsn := os.Getenv("KAVE_TEST_V2_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_V2_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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
		t.Fatal(err)
	}
	store, err := NewApplyStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}

	for _, status := range []string{"active", "disabled"} {
		t.Run(status, func(t *testing.T) {
			account := corev2.Ref(ids.New("acc"))
			existingID := corev2.Ref(ids.New("nsp"))
			ns := corev2.Namespace{
				Account: account, Application: corev2.Ref("existing-" + status + "-" + ids.New("app")), Environment: "test",
			}
			if existingID == deterministicNamespaceID(ns) {
				t.Fatal("test namespace ID unexpectedly matched deterministic ID")
			}
			scope := Scope{AccountID: string(account), NamespaceID: string(existingID)}
			if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
				_, err := db.Exec(ctx, `
INSERT INTO kave_v2.namespaces (id, account_id, application, environment, status)
VALUES ($1, $2, $3, $4, $5)
`, existingID, account, ns.Application, ns.Environment, status)
				return err
			}); err != nil {
				t.Fatalf("seed preexisting namespace: %v", err)
			}

			result, applyErr := store.Apply(ctx, corev2.ApplyRequest{
				Caller: corev2.Caller{
					AccountID: account, ServiceKeyID: "offline-bootstrap",
					Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true,
				},
				Manifest: corev2.Manifest{Namespace: ns}, IdempotencyKey: corev2.Ref("existing/" + status),
			})
			if status == "disabled" {
				if !errors.Is(applyErr, corev2.ErrUnauthorized) {
					t.Fatalf("Apply disabled preexisting namespace error = %v, want unauthorized", applyErr)
				}
				return
			}
			if applyErr != nil {
				t.Fatalf("Apply active preexisting namespace: %v", applyErr)
			}
			if result.NamespaceID != existingID || result.Revision != 1 {
				t.Fatalf("preexisting Apply result = %+v, want namespace %q revision 1", result, existingID)
			}
			serviceKeyID := corev2.Ref(ids.New("key"))
			if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
				_, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys (
    id, account_id, namespace_id, name, lookup_prefix, secret_hash, capabilities
) VALUES ($1, $2, $3, 'config-admin', $4, $5, ARRAY['config.apply'])
`, serviceKeyID, account, existingID, "lookup_"+ids.New("pfx"), make([]byte, 32))
				return err
			}); err != nil {
				t.Fatalf("seed namespace config key: %v", err)
			}
			standingCaller := corev2.Caller{
				AccountID: account, NamespaceID: existingID, ServiceKeyID: serviceKeyID,
				Operations: []corev2.Operation{corev2.OperationApply},
			}
			standingResult, err := store.Apply(ctx, corev2.ApplyRequest{
				Caller: standingCaller, Manifest: corev2.Manifest{Namespace: ns},
				IdempotencyKey: "existing/standing-key",
			})
			if err != nil {
				t.Fatalf("namespace-scoped Apply: %v", err)
			}
			if standingResult.NamespaceID != existingID {
				t.Fatalf("namespace-scoped Apply resolved %q, want %q", standingResult.NamespaceID, existingID)
			}
			other := ns
			other.Application = corev2.Ref("other-" + ids.New("app"))
			if _, err := store.Apply(ctx, corev2.ApplyRequest{
				Caller: standingCaller, Manifest: corev2.Manifest{Namespace: other},
				IdempotencyKey: "existing/cross-namespace",
			}); !errors.Is(err, corev2.ErrUnauthorized) {
				t.Fatalf("cross-namespace standing Apply error = %v, want unauthorized", err)
			}
			if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
				var count int64
				if err := db.QueryRow(ctx, `
SELECT count(*) FROM kave_v2.namespaces
WHERE account_id = $1 AND application = $2 AND environment = $3
`, account, ns.Application, ns.Environment).Scan(&count); err != nil {
					return err
				}
				if count != 1 {
					return fmt.Errorf("namespace tuple count = %d, want 1", count)
				}
				return nil
			}); err != nil {
				t.Fatalf("verify preexisting namespace uniqueness: %v", err)
			}
		})
	}
}

func TestApplyStorePostgres_DisabledNamespaceCannotBeReenabled(t *testing.T) {
	dsn := os.Getenv("KAVE_TEST_V2_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_V2_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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
		t.Fatal(err)
	}
	store, err := NewApplyStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	account := corev2.Ref(ids.New("acc"))
	ns := corev2.Namespace{
		Account: account, Application: corev2.Ref("disabled-" + ids.New("app")), Environment: "test",
	}
	caller := corev2.Caller{
		AccountID: account, ServiceKeyID: "offline-bootstrap",
		Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true,
	}
	created, err := store.Apply(ctx, corev2.ApplyRequest{
		Caller: caller, Manifest: corev2.Manifest{Namespace: ns}, IdempotencyKey: "disabled/bootstrap",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{AccountID: string(account), NamespaceID: string(created.NamespaceID)}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		_, err := db.Exec(ctx, `
UPDATE kave_v2.namespaces
SET status = 'disabled'
WHERE account_id = $1 AND id = $2
`, account, created.NamespaceID)
		return err
	}); err != nil {
		t.Fatalf("disable namespace: %v", err)
	}

	_, err = store.Apply(ctx, corev2.ApplyRequest{
		Caller: caller, Manifest: corev2.Manifest{Namespace: ns}, IdempotencyKey: "disabled/reapply",
	})
	if !errors.Is(err, corev2.ErrUnauthorized) {
		t.Fatalf("Apply disabled namespace error = %v, want unauthorized", err)
	}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		var status string
		if err := db.QueryRow(ctx, `
SELECT status FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
`, account, created.NamespaceID).Scan(&status); err != nil {
			return err
		}
		if status != "disabled" {
			return fmt.Errorf("namespace status = %q, want disabled", status)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func countChanges(changes []corev2.Change, kind corev2.ChangeKind) int {
	count := 0
	for _, change := range changes {
		if change.Kind == kind {
			count++
		}
	}
	return count
}

func assertApplyStatuses(t *testing.T, ctx context.Context, runner *ScopedRunner, scope Scope, limitEnabled, limitSuperseded bool, agentStatus, routeStatus string) {
	t.Helper()
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		var gotLimit, gotSuperseded bool
		var gotAgent, gotRoute string
		if err := db.QueryRow(ctx, `
SELECT
    COALESCE((
    SELECT enabled FROM kave_v2.limits
    WHERE account_id = $1 AND namespace_id = $2
      AND external_key = 'monthly-actions' AND superseded_at IS NULL
), FALSE),
    EXISTS (
        SELECT 1 FROM kave_v2.limits
        WHERE account_id = $1 AND namespace_id = $2
          AND external_key = 'monthly-actions' AND superseded_at IS NOT NULL
    )
`, scope.AccountID, scope.NamespaceID).Scan(&gotLimit, &gotSuperseded); err != nil {
			return err
		}
		if err := db.QueryRow(ctx, `
SELECT status FROM kave_v2.agents
WHERE account_id = $1 AND namespace_id = $2 AND name = 'assistant'
`, scope.AccountID, scope.NamespaceID).Scan(&gotAgent); err != nil {
			return err
		}
		if err := db.QueryRow(ctx, `
SELECT status FROM kave_v2.provider_routes
WHERE account_id = $1 AND namespace_id = $2 AND name = 'openai'
`, scope.AccountID, scope.NamespaceID).Scan(&gotRoute); err != nil {
			return err
		}
		if gotLimit != limitEnabled || gotSuperseded != limitSuperseded || gotAgent != agentStatus || gotRoute != routeStatus {
			return fmt.Errorf("statuses = %t/%t/%s/%s, want %t/%t/%s/%s", gotLimit, gotSuperseded, gotAgent, gotRoute, limitEnabled, limitSuperseded, agentStatus, routeStatus)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
