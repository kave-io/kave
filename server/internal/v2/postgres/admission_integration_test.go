package postgres_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
)

func TestAdmissionStorePostgres_AtomicCapAndIdempotency(t *testing.T) {
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

	migrator, err := v2postgres.NewMigrator(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	fixture := seedAdmissionFixture(t, ctx, pool, 5)
	store, err := v2postgres.NewAdmissionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	service := corev2.NewAdmissionService(store)

	var admitted atomic.Int32
	var rejected atomic.Int32
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := fixture.request(corev2.Ref(fmt.Sprintf("run/%d", i)))
			_, err := service.Consume(ctx, req)
			switch {
			case err == nil:
				admitted.Add(1)
			case errors.Is(err, corev2.ErrLimitExceeded):
				rejected.Add(1)
			default:
				t.Errorf("consume: %v", err)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 5 || rejected.Load() != 15 {
		t.Fatalf("admitted=%d rejected=%d, want 5/15", admitted.Load(), rejected.Load())
	}

	var used, usageRows, invocationRows, logicalRows, appliedRows, blockedRows, reconstructed int64
	if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		if err := db.QueryRow(ctx, `SELECT used FROM kave_v2.limit_windows WHERE limit_id = $1`, fixture.limitID).Scan(&used); err != nil {
			return err
		}
		if err := db.QueryRow(ctx, `
SELECT count(*) FROM kave_v2.usage_entries
WHERE account_id = $1 AND namespace_id = $2 AND metric = 'ai_actions'
`, fixture.accountID, fixture.namespaceID).Scan(&usageRows); err != nil {
			return err
		}
		if err := db.QueryRow(ctx, `
SELECT
    count(*) FILTER (WHERE usage_detail->>'entry_role' = 'logical'),
    count(*) FILTER (WHERE event_kind = 'consume' AND limit_id = $3),
    count(*) FILTER (WHERE event_kind = 'block' AND limit_id = $3),
    COALESCE(sum(quantity) FILTER (WHERE event_kind = 'consume' AND limit_id = $3), 0)
FROM kave_v2.usage_entries
WHERE account_id = $1 AND namespace_id = $2
`, fixture.accountID, fixture.namespaceID, fixture.limitID).
			Scan(&logicalRows, &appliedRows, &blockedRows, &reconstructed); err != nil {
			return err
		}
		return db.QueryRow(ctx, `
SELECT count(*) FROM kave_v2.invocations
WHERE account_id = $1 AND namespace_id = $2 AND operation = 'consume'
`, fixture.accountID, fixture.namespaceID).Scan(&invocationRows)
	}); err != nil {
		t.Fatal(err)
	}
	if used != 5 || usageRows != 25 || invocationRows != 20 ||
		logicalRows != 5 || appliedRows != 5 || blockedRows != 15 || reconstructed != used {
		t.Fatalf("used=%d usage=%d invocations=%d logical/applied/blocked/reconstructed=%d/%d/%d/%d",
			used, usageRows, invocationRows, logicalRows, appliedRows, blockedRows, reconstructed)
	}

	// Replaying an admitted key returns its original decision without a new row
	// or counter increment.
	first, err := service.Consume(ctx, fixture.request("replay/one"))
	if !errors.Is(err, corev2.ErrLimitExceeded) {
		// The cap is already full, so the first decision is a stable rejection.
		t.Fatalf("first replay decision: %v", err)
	}
	second, err := service.Consume(ctx, fixture.request("replay/one"))
	if !errors.Is(err, corev2.ErrLimitExceeded) {
		t.Fatalf("second replay decision: %v", err)
	}
	if !second.Replayed || first.InvocationID != second.InvocationID {
		t.Fatalf("rejected replay mismatch: first=%+v second=%+v", first, second)
	}
}

func TestAdmissionStorePostgres_IdempotencySurvivesServiceKeyRotation(t *testing.T) {
	dsn := os.Getenv("KAVE_TEST_V2_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_V2_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	migrator, _ := v2postgres.NewMigrator(pool)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	fixture := seedAdmissionFixture(t, ctx, pool, 2)
	store, _ := v2postgres.NewAdmissionStore(pool)
	service := corev2.NewAdmissionService(store)
	req := fixture.request("run/rotation-safe")
	first, err := service.Consume(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	rotatedKeyID := corev2.Ref(ids.New("key"))
	verifier := sha256.Sum256([]byte("rotated-fixture-key"))
	if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys (
    id, account_id, namespace_id, name, lookup_prefix, secret_hash,
    capabilities, allowed_agent_ids, can_assert_scope
) VALUES ($1, $2, $3, 'rotated-worker', $4, $5, ARRAY['consume'], ARRAY[$6], TRUE)
`, rotatedKeyID, fixture.accountID, fixture.namespaceID,
			"lookup_"+ids.New("pfx"), verifier[:], fixture.agentID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	req.Caller.ServiceKeyID = rotatedKeyID
	replayed, err := service.Consume(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.InvocationID != first.InvocationID {
		t.Fatalf("rotation replay = %+v, first = %+v", replayed, first)
	}
	otherAgentID, otherKeyID := corev2.Ref(ids.New("agt")), corev2.Ref(ids.New("key"))
	otherVerifier := sha256.Sum256([]byte("other-agent-key"))
	if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		var routeID string
		if err := db.QueryRow(ctx, `SELECT route_id FROM kave_v2.agents WHERE id = $1`, fixture.agentID).Scan(&routeID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.agents (id, account_id, namespace_id, name, kind, route_id)
VALUES ($1, $2, $3, 'other-agent', 'llm', $4)
`, otherAgentID, fixture.accountID, fixture.namespaceID, routeID); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys (
    id, account_id, namespace_id, name, lookup_prefix, secret_hash,
    capabilities, allowed_agent_ids, can_assert_scope
) VALUES ($1, $2, $3, 'other-worker', $4, $5, ARRAY['consume'], ARRAY[$6], TRUE)
`, otherKeyID, fixture.accountID, fixture.namespaceID,
			"lookup_"+ids.New("pfx"), otherVerifier[:], otherAgentID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	unauthorizedReplay := req
	unauthorizedReplay.Caller.ServiceKeyID = otherKeyID
	unauthorizedReplay.Caller.AllowedAgentIDs = []corev2.Ref{otherAgentID}
	if _, err := service.Consume(ctx, unauthorizedReplay); !errors.Is(err, corev2.ErrUnauthorized) {
		t.Fatalf("cross-agent idempotency replay error = %v", err)
	}
	var used, invocations int64
	if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		if err := db.QueryRow(ctx, `SELECT used FROM kave_v2.limit_windows WHERE limit_id = $1`, fixture.limitID).Scan(&used); err != nil {
			return err
		}
		return db.QueryRow(ctx, `
SELECT count(*) FROM kave_v2.invocations
WHERE account_id = $1 AND namespace_id = $2 AND idempotency_key = 'run/rotation-safe'
`, fixture.accountID, fixture.namespaceID).Scan(&invocations)
	}); err != nil {
		t.Fatal(err)
	}
	if used != 1 || invocations != 1 {
		t.Fatalf("used/invocations = %d/%d, want 1/1", used, invocations)
	}
}

type admissionFixture struct {
	runner       *v2postgres.ScopedRunner
	scope        v2postgres.Scope
	accountID    corev2.Ref
	namespaceID  corev2.Ref
	serviceKeyID corev2.Ref
	agentID      corev2.Ref
	agentName    corev2.Ref
	limitID      string
}

func (f admissionFixture) request(idempotency corev2.Ref) corev2.ConsumeRequest {
	return corev2.ConsumeRequest{
		Caller: corev2.Caller{
			AccountID: f.accountID, NamespaceID: f.namespaceID, ServiceKeyID: f.serviceKeyID,
			Operations: []corev2.Operation{corev2.OperationConsume}, AllowedAgentIDs: []corev2.Ref{f.agentID}, CanAssertScope: true,
		},
		Agent:  f.agentName,
		Scope:  corev2.Scope{Tenant: "clinic/a", BillTo: "clinic/a", Feature: "ai_actions"},
		Metric: "ai_actions", Units: 1, IdempotencyKey: idempotency,
	}
}

func seedAdmissionFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, hardCap int64) admissionFixture {
	t.Helper()
	runner, err := v2postgres.NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	accountID := corev2.Ref(ids.New("acc"))
	namespaceID := corev2.Ref(ids.New("nsp"))
	serviceKeyID := corev2.Ref(ids.New("key"))
	agentID := corev2.Ref(ids.New("agt"))
	routeID := ids.New("rte")
	limitID := ids.New("lim")
	scope := v2postgres.Scope{AccountID: string(accountID), NamespaceID: string(namespaceID)}
	hash := sha256.Sum256([]byte("fixture-key"))

	err = runner.WithScope(ctx, scope, func(ctx context.Context, db v2postgres.DBTX) error {
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.namespaces (id, account_id, application, environment)
VALUES ($1, $2, 'test-app', 'test-env')
`, namespaceID, accountID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.provider_routes (
    id, account_id, namespace_id, name, provider, base_url
) VALUES ($1, $2, $3, 'openai-primary', 'openai', 'https://api.openai.com')
`, routeID, accountID, namespaceID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.agents (
    id, account_id, namespace_id, name, kind, route_id
) VALUES ($1, $2, $3, 'clinic-assistant', 'llm', $4)
`, agentID, accountID, namespaceID, routeID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys (
    id, account_id, namespace_id, name, lookup_prefix, secret_hash,
    capabilities, allowed_agent_ids, can_assert_scope
) VALUES ($1, $2, $3, 'worker', $4, $5, ARRAY['consume'], ARRAY[$6], TRUE)
`, serviceKeyID, accountID, namespaceID, "lookup_"+ids.New("pfx"), hash[:], agentID); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.limits (
    id, account_id, namespace_id, external_key, metric,
    tenant_ref, agent_id, hard_cap, window_kind
) VALUES ($1, $2, $3, 'clinic-actions', 'ai_actions', 'clinic/a', $4, $5, 'calendar_month')
`, limitID, accountID, namespaceID, agentID, hardCap)
		return err
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return admissionFixture{
		runner: runner, scope: scope, accountID: accountID, namespaceID: namespaceID,
		serviceKeyID: serviceKeyID, agentID: agentID, agentName: "clinic-assistant", limitID: limitID,
	}
}
