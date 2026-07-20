package postgres_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
)

func TestLimitPolicyApplyConcurrentWithAdmissionPreservesSingleCounter(t *testing.T) {
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
		t.Fatal(err)
	}

	fixture := seedAdmissionFixture(t, ctx, pool, 100)
	secretID := ids.New("sec")
	if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.secrets (
    id, account_id, namespace_id, name, backend, ciphertext, wrapping_key_id
) VALUES ($1, $2, $3, 'provider', 'encrypted', decode('00', 'hex'), 'test-key')
`, secretID, fixture.accountID, fixture.namespaceID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	manifest := corev2.Manifest{
		Namespace: corev2.Namespace{
			Account: fixture.accountID, Application: "test-app", Environment: "test-env",
		},
		Routes: []corev2.RouteSpec{{
			Name: "openai-primary", Provider: "openai", BaseURL: "https://api.openai.com",
			Secret: "provider", AllowedModels: []string{"test-model"}, DefaultModel: "test-model",
			PricingRevision: 2, Pricing: []corev2.ModelPrice{{Model: "test-model"}},
		}},
		Agents: []corev2.AgentSpec{{
			Name: fixture.agentName, Kind: corev2.AgentLLM, Route: "openai-primary", Enabled: true,
		}},
		Limits: []corev2.LimitSpec{{
			Key: "clinic-actions", Metric: "ai_actions",
			Selector: corev2.LimitSelector{Tenant: "clinic/a", Agent: fixture.agentName},
			Window:   corev2.WindowMonth, HardCap: 100, Enabled: true,
		}},
	}
	applyStore, err := v2postgres.NewApplyStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	applyCaller := corev2.Caller{
		AccountID: fixture.accountID, ServiceKeyID: "bootstrap",
		Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true,
	}
	configured, err := applyStore.Apply(ctx, corev2.ApplyRequest{
		Caller: applyCaller, Manifest: manifest, IdempotencyKey: "policy-concurrency/configure",
	})
	if err != nil {
		t.Fatal(err)
	}

	admissionStore, err := v2postgres.NewAdmissionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	admission := corev2.NewAdmissionService(admissionStore)
	manifest.Limits[0].HardCap = 200

	const consumes = 25
	start := make(chan struct{})
	errs := make(chan error, consumes+1)
	var wg sync.WaitGroup
	for i := range consumes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := admission.Consume(ctx, fixture.request(corev2.Ref(fmt.Sprintf("policy-concurrency/%d", i))))
			errs <- err
		}()
	}
	var updated corev2.ApplyResult
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		var err error
		updated, err = applyStore.Apply(ctx, corev2.ApplyRequest{
			Caller: applyCaller, Manifest: manifest, ExpectedRevision: configured.Revision,
			IdempotencyKey: "policy-concurrency/cap-200",
		})
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent admission/apply: %v", err)
		}
	}
	if !updated.Applied || updated.Revision != configured.Revision+1 {
		t.Fatalf("policy Apply result = %+v", updated)
	}

	if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		var currentID string
		var generations, archived, hardCap, used, reserved int64
		if err := db.QueryRow(ctx, `
SELECT
    max(id) FILTER (WHERE superseded_at IS NULL),
    count(*),
    count(*) FILTER (WHERE superseded_at IS NOT NULL),
    max(hard_cap) FILTER (WHERE superseded_at IS NULL)
FROM kave_v2.limits
WHERE account_id = $1 AND namespace_id = $2 AND external_key = 'clinic-actions'
`, fixture.accountID, fixture.namespaceID).
			Scan(&currentID, &generations, &archived, &hardCap); err != nil {
			return err
		}
		if err := db.QueryRow(ctx, `
SELECT used, reserved
FROM kave_v2.limit_windows
WHERE account_id = $1 AND namespace_id = $2 AND limit_id = $3
`, fixture.accountID, fixture.namespaceID, currentID).Scan(&used, &reserved); err != nil {
			return err
		}
		if currentID != fixture.limitID || generations != 1 || archived != 0 ||
			hardCap != 200 || used != consumes || reserved != 0 {
			return fmt.Errorf("limit identity/generations/cap/counter = %q %d/%d %d %d/%d, want %q 1/0 200 %d/0",
				currentID, generations, archived, hardCap, used, reserved, fixture.limitID, consumes)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
