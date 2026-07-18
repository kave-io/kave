package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

func TestProviderStorePostgres_ReserveSettleAndIdempotency(t *testing.T) {
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
	migrator, _ := v2postgres.NewMigrator(pool)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(100 + i)
	}
	cipher, err := v2postgres.NewLocalEnvelopeCipher(hex.EncodeToString(master))
	if err != nil {
		t.Fatal(err)
	}
	fixture := seedProviderFixture(t, ctx, pool, cipher)
	store, err := v2postgres.NewProviderStore(pool, cipher)
	if err != nil {
		t.Fatal(err)
	}

	request := fixture.request("invoke/one")
	grant, err := store.Begin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if string(grant.Credential) != "provider-secret" || grant.Model != "gpt-safe" || grant.Price == nil {
		t.Fatalf("grant = %#v", grant)
	}
	if _, err := store.Begin(ctx, request); !errors.Is(err, provider.ErrInvocationInProgress) {
		t.Fatalf("in-progress duplicate error = %v", err)
	}
	conflict := request
	conflict.RequestHash = sha256.Sum256([]byte("different"))
	if _, err := store.Begin(ctx, conflict); !errors.Is(err, corev2.ErrIdempotencyConflict) {
		t.Fatalf("conflict error = %v", err)
	}

	if err := store.StartAttempt(ctx, provider.AttemptRequest{Grant: grant, AttemptNo: 1, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, provider.CompleteRequest{
		Grant: grant, AttemptNo: 1, HTTPStatus: 200, Usage: provider.Usage{
			InputTokens: 10, OutputTokens: 3, CostNanos: 32, Currency: "USD", Model: "gpt-safe", Reported: true,
		}, FinishedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(ctx, request); !errors.Is(err, provider.ErrAlreadyInvoked) {
		t.Fatalf("settled duplicate error = %v", err)
	}

	second, err := store.Begin(ctx, fixture.request("invoke/two"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, provider.CompleteRequest{Grant: second, FinishedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	third, err := store.Begin(ctx, fixture.request("invoke/three"))
	if err != nil {
		t.Fatalf("released request reservation was not reusable: %v", err)
	}

	var requestUsed, requestReserved, inputUsed, inputReserved, outputUsed, outputReserved, costUsed, costReserved int64
	err = fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		return db.QueryRow(ctx, `
SELECT
  MAX(used) FILTER (WHERE l.metric = 'requests'), MAX(reserved) FILTER (WHERE l.metric = 'requests'),
  MAX(used) FILTER (WHERE l.metric = 'input_tokens'), MAX(reserved) FILTER (WHERE l.metric = 'input_tokens'),
  MAX(used) FILTER (WHERE l.metric = 'output_tokens'), MAX(reserved) FILTER (WHERE l.metric = 'output_tokens'),
  MAX(used) FILTER (WHERE l.metric = 'cost_nano_usd'), MAX(reserved) FILTER (WHERE l.metric = 'cost_nano_usd')
FROM kave_v2.limit_windows w JOIN kave_v2.limits l ON l.id = w.limit_id
WHERE w.account_id = $1 AND w.namespace_id = $2
`, fixture.accountID, fixture.namespaceID).Scan(
			&requestUsed, &requestReserved, &inputUsed, &inputReserved,
			&outputUsed, &outputReserved, &costUsed, &costReserved,
		)
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestUsed != 1 || requestReserved != 1 || inputUsed != 10 || inputReserved != 100 || outputUsed != 3 || outputReserved != 5 || costUsed != 32 || costReserved != 220 {
		t.Fatalf("counters request=%d/%d input=%d/%d output=%d/%d cost=%d/%d", requestUsed, requestReserved, inputUsed, inputReserved, outputUsed, outputReserved, costUsed, costReserved)
	}
	if err := store.Complete(ctx, provider.CompleteRequest{Grant: third, FinishedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	unbounded := fixture.request("invoke/unbounded")
	unbounded.OutputBounded = false
	if _, err := store.Begin(ctx, unbounded); !errors.Is(err, provider.ErrReservationUnavailable) {
		t.Fatalf("unbounded output error = %v", err)
	}
}

func TestProviderStorePostgres_ObservedUsageOverReservationIsAccounted(t *testing.T) {
	ctx, pool, cipher := providerPostgresTest(t)
	fixture := seedProviderFixture(t, ctx, pool, cipher)
	store, err := v2postgres.NewProviderStore(pool, cipher)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := store.Begin(ctx, fixture.request("invoke/observed-overage"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartAttempt(ctx, provider.AttemptRequest{
		Grant: grant, AttemptNo: grant.AttemptNo, StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, provider.CompleteRequest{
		Grant: grant, AttemptNo: grant.AttemptNo, HTTPStatus: 200,
		Usage: provider.Usage{
			InputTokens: 10, OutputTokens: 7, CostNanos: 48, Currency: "USD",
			Model: "gpt-safe", Reported: true,
		},
		FinishedAt: time.Now(),
	}); err != nil {
		t.Fatalf("settle observed overage: %v", err)
	}
	var used, reserved int64
	if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		return db.QueryRow(ctx, `
SELECT w.used, w.reserved
FROM kave_v2.limit_windows AS w
JOIN kave_v2.limits AS l ON l.id = w.limit_id
WHERE w.account_id = $1 AND w.namespace_id = $2
  AND l.metric = 'output_tokens'
`, fixture.accountID, fixture.namespaceID).Scan(&used, &reserved)
	}); err != nil {
		t.Fatal(err)
	}
	if used != 7 || reserved != 0 {
		t.Fatalf("output counter = used %d reserved %d, want 7/0", used, reserved)
	}
}

func TestProviderStorePostgres_RetryFailedAttemptAfterServiceKeyRotation(t *testing.T) {
	ctx, pool, cipher := providerPostgresTest(t)
	fixture := seedProviderFixture(t, ctx, pool, cipher)
	store, _ := v2postgres.NewProviderStore(pool, cipher)
	request := fixture.request("invoke/retry")
	first, err := store.Begin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.AttemptNo != 1 {
		t.Fatalf("first attempt = %d", first.AttemptNo)
	}
	if err := store.StartAttempt(ctx, provider.AttemptRequest{Grant: first, AttemptNo: first.AttemptNo, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, provider.CompleteRequest{Grant: first, AttemptNo: first.AttemptNo, Uncertain: true, FinishedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	rotated := fixture.addServiceKey(t, ctx)
	request.Caller.ServiceKeyID = rotated
	second, err := store.Begin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.InvocationID != first.InvocationID || second.AttemptNo != 2 || second.ServiceKeyID != rotated {
		t.Fatalf("retry grant = %#v, first = %#v", second, first)
	}
	if err := store.Complete(ctx, provider.CompleteRequest{Grant: second, FinishedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestProviderStorePostgres_ExpiredLeaseRecoveryIsConservative(t *testing.T) {
	for _, deliveryStarted := range []bool{false, true} {
		t.Run(map[bool]string{false: "before egress", true: "after egress"}[deliveryStarted], func(t *testing.T) {
			ctx, pool, cipher := providerPostgresTest(t)
			fixture := seedProviderFixture(t, ctx, pool, cipher)
			store, _ := v2postgres.NewProviderStore(pool, cipher)
			request := fixture.request("invoke/expired")
			first, err := store.Begin(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if deliveryStarted {
				if err := store.StartAttempt(ctx, provider.AttemptRequest{Grant: first, AttemptNo: first.AttemptNo, StartedAt: time.Now()}); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.RenewLease(ctx, first); err != nil {
				t.Fatalf("renew lease: %v", err)
			}
			if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
				_, err := db.Exec(ctx, `UPDATE kave_v2.invocations SET lease_expires_at = $4 WHERE account_id = $1 AND namespace_id = $2 AND id = $3`, fixture.accountID, fixture.namespaceID, first.InvocationID, time.Now().Add(-time.Minute))
				return err
			}); err != nil {
				t.Fatal(err)
			}

			second, err := store.Begin(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if second.InvocationID != first.InvocationID || second.AttemptNo != 2 {
				t.Fatalf("recovered grant = %#v", second)
			}
			var used, reserved int64
			var reportedRequests, reportedInput, reportedOutput, reportedCost int64
			var reportedProvider, reportedModel string
			var reportedEstimate bool
			if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
				if err := db.QueryRow(ctx, `
SELECT used, reserved FROM kave_v2.limit_windows w
JOIN kave_v2.limits l ON l.id = w.limit_id
WHERE w.account_id = $1 AND w.namespace_id = $2 AND l.metric = 'requests'
`, fixture.accountID, fixture.namespaceID).Scan(&used, &reserved); err != nil {
					return err
				}
				return db.QueryRow(ctx, `
	SELECT request_count, input_tokens, output_tokens, cost_nanos,
	       COALESCE(provider, ''), COALESCE(model, ''),
	       COALESCE((usage_detail->>'accounting_estimate')::BOOLEAN, FALSE)
FROM kave_v2.usage_entries
WHERE account_id = $1 AND namespace_id = $2 AND invocation_id = $3
  AND dedupe_key = $4
`, fixture.accountID, fixture.namespaceID, first.InvocationID,
					"usage:"+first.InvocationID+":1").Scan(
					&reportedRequests, &reportedInput, &reportedOutput, &reportedCost,
					&reportedProvider, &reportedModel, &reportedEstimate,
				)
			}); err != nil {
				t.Fatal(err)
			}
			wantUsed := int64(0)
			if deliveryStarted {
				wantUsed = 1
			}
			if used != wantUsed || reserved != 1 {
				t.Fatalf("request counter = used %d reserved %d, want %d/1", used, reserved, wantUsed)
			}
			wantInput, wantOutput, wantCost := int64(0), int64(0), int64(0)
			if deliveryStarted {
				wantInput, wantOutput, wantCost = 100, 5, 220
			}
			if reportedRequests != wantUsed || reportedInput != wantInput ||
				reportedOutput != wantOutput || reportedCost != wantCost ||
				reportedProvider != "openai" || reportedModel != "gpt-safe" || reportedEstimate != deliveryStarted {
				t.Fatalf("recovered canonical usage = %d/%d/%d/%d %q/%q estimated=%v, want %d/%d/%d/%d openai/gpt-safe estimated=%v",
					reportedRequests, reportedInput, reportedOutput, reportedCost,
					reportedProvider, reportedModel, reportedEstimate,
					wantUsed, wantInput, wantOutput, wantCost, deliveryStarted)
			}
			if err := store.Complete(ctx, provider.CompleteRequest{Grant: second, FinishedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProviderStorePostgres_DifferentInvocationReconcilesExpiredLease(t *testing.T) {
	for _, deliveryStarted := range []bool{false, true} {
		t.Run(map[bool]string{false: "release before egress", true: "charge after egress"}[deliveryStarted], func(t *testing.T) {
			ctx, pool, cipher := providerPostgresTest(t)
			fixture := seedProviderFixture(t, ctx, pool, cipher)
			store, err := v2postgres.NewProviderStore(pool, cipher)
			if err != nil {
				t.Fatal(err)
			}

			orphan, err := store.Begin(ctx, fixture.request("invoke/orphan"))
			if err != nil {
				t.Fatal(err)
			}
			if deliveryStarted {
				if err := store.StartAttempt(ctx, provider.AttemptRequest{
					Grant: orphan, AttemptNo: orphan.AttemptNo, StartedAt: time.Now(),
				}); err != nil {
					t.Fatal(err)
				}
				// Force a persistence error at the end of settlement. The
				// surrounding transaction must roll back every counter update and
				// leave the reservation recoverable after its lease expires.
				if err := store.Complete(ctx, provider.CompleteRequest{
					Grant: orphan, AttemptNo: orphan.AttemptNo, HTTPStatus: 200,
					Usage: provider.Usage{
						InputTokens: 1, OutputTokens: 1, CostNanos: 1,
						Currency: "invalid", Model: "gpt-safe", Reported: true,
					},
					FinishedAt: time.Now(),
				}); err == nil {
					t.Fatal("invalid settlement unexpectedly succeeded")
				}
			}
			if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
				_, err := db.Exec(ctx, `
UPDATE kave_v2.invocations
SET lease_expires_at = $4
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, fixture.accountID, fixture.namespaceID, orphan.InvocationID, time.Now().Add(-time.Minute))
				return err
			}); err != nil {
				t.Fatal(err)
			}

			// A different idempotency key must make progress without the orphan's
			// key ever being retried. Begin performs a bounded namespace-scoped
			// recovery before it evaluates this request's reservations.
			fresh, err := store.Begin(ctx, fixture.request("invoke/fresh"))
			if err != nil {
				t.Fatalf("different invocation remained wedged by orphan: %v", err)
			}
			if fresh.InvocationID == orphan.InvocationID || fresh.AttemptNo != 1 {
				t.Fatalf("fresh grant = %#v, orphan = %#v", fresh, orphan)
			}

			var orphanStatus string
			var used, reserved int64
			if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
				if err := db.QueryRow(ctx, `
SELECT status
FROM kave_v2.invocations
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, fixture.accountID, fixture.namespaceID, orphan.InvocationID).Scan(&orphanStatus); err != nil {
					return err
				}
				return db.QueryRow(ctx, `
SELECT used, reserved
FROM kave_v2.limit_windows AS w
JOIN kave_v2.limits AS l ON l.id = w.limit_id
WHERE w.account_id = $1 AND w.namespace_id = $2 AND l.metric = 'requests'
`, fixture.accountID, fixture.namespaceID).Scan(&used, &reserved)
			}); err != nil {
				t.Fatal(err)
			}
			wantUsed := int64(0)
			if deliveryStarted {
				wantUsed = 1
			}
			if orphanStatus != "failed" || used != wantUsed || reserved != 1 {
				t.Fatalf("orphan status/counter = %s %d/%d, want failed %d/1", orphanStatus, used, reserved, wantUsed)
			}
			if err := store.Complete(ctx, provider.CompleteRequest{Grant: fresh, FinishedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProviderStorePostgres_ConcurrentRecoverySettlesOrphanOnce(t *testing.T) {
	ctx, pool, cipher := providerPostgresTest(t)
	fixture := seedProviderFixture(t, ctx, pool, cipher)
	store, err := v2postgres.NewProviderStore(pool, cipher)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := store.Begin(ctx, fixture.request("invoke/concurrent-orphan"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartAttempt(ctx, provider.AttemptRequest{
		Grant: orphan, AttemptNo: orphan.AttemptNo, StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		_, err := db.Exec(ctx, `
UPDATE kave_v2.invocations
SET lease_expires_at = $4
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, fixture.accountID, fixture.namespaceID, orphan.InvocationID, time.Now().Add(-time.Minute))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	type beginResult struct {
		grant provider.Grant
		err   error
	}
	start := make(chan struct{})
	results := make(chan beginResult, 2)
	for _, key := range []corev2.Ref{"invoke/concurrent-a", "invoke/concurrent-b"} {
		request := fixture.request(key)
		go func() {
			<-start
			grant, err := store.Begin(ctx, request)
			results <- beginResult{grant: grant, err: err}
		}()
	}
	close(start)
	var succeeded int
	var admitted provider.Grant
	for range 2 {
		result := <-results
		if result.err == nil {
			succeeded++
			admitted = result.grant
			continue
		}
		if !errors.Is(result.err, corev2.ErrLimitExceeded) {
			t.Fatalf("concurrent admission error = %v", result.err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful concurrent admissions = %d, want 1", succeeded)
	}

	var used, reserved, requestSettlements int64
	if err := fixture.runner.WithScope(ctx, fixture.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		if err := db.QueryRow(ctx, `
SELECT used, reserved
FROM kave_v2.limit_windows AS w
JOIN kave_v2.limits AS l ON l.id = w.limit_id
WHERE w.account_id = $1 AND w.namespace_id = $2 AND l.metric = 'requests'
`, fixture.accountID, fixture.namespaceID).Scan(&used, &reserved); err != nil {
			return err
		}
		return db.QueryRow(ctx, `
SELECT count(*)
FROM kave_v2.usage_entries
WHERE account_id = $1 AND namespace_id = $2 AND invocation_id = $3
  AND event_kind = 'settlement' AND metric = 'requests'
`, fixture.accountID, fixture.namespaceID, orphan.InvocationID).Scan(&requestSettlements)
	}); err != nil {
		t.Fatal(err)
	}
	if used != 1 || reserved != 1 || requestSettlements != 1 {
		t.Fatalf("counter/settlements = %d/%d/%d, want 1/1/1", used, reserved, requestSettlements)
	}
	if err := store.Complete(ctx, provider.CompleteRequest{Grant: admitted, FinishedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func providerPostgresTest(t *testing.T) (context.Context, *pgxpool.Pool, v2postgres.SecretCipher) {
	t.Helper()
	dsn := os.Getenv("KAVE_TEST_V2_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_V2_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrator, _ := v2postgres.NewMigrator(pool)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	master := sha256.Sum256([]byte(t.Name()))
	cipher, err := v2postgres.NewLocalEnvelopeCipher(hex.EncodeToString(master[:]))
	if err != nil {
		t.Fatal(err)
	}
	return ctx, pool, cipher
}

type providerFixture struct {
	runner                                        *v2postgres.ScopedRunner
	scope                                         v2postgres.Scope
	accountID, namespaceID, serviceKeyID, agentID corev2.Ref
}

func (f providerFixture) addServiceKey(t *testing.T, ctx context.Context) corev2.Ref {
	t.Helper()
	id := corev2.Ref(ids.New("key"))
	hash := sha256.Sum256([]byte(id))
	err := f.runner.WithScope(ctx, f.scope, func(ctx context.Context, db v2postgres.DBTX) error {
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys (
  id,account_id,namespace_id,name,lookup_prefix,secret_hash,capabilities,allowed_agent_ids,can_assert_scope
) VALUES ($1,$2,$3,$4,$5,$6,ARRAY['invoke'],ARRAY[$7],TRUE)
`, id, f.accountID, f.namespaceID, "rotated-"+string(id), "lookup_"+ids.New("pfx"), hash[:], f.agentID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f providerFixture) request(key corev2.Ref) provider.BeginRequest {
	return provider.BeginRequest{
		Caller: corev2.Caller{
			AccountID: f.accountID, NamespaceID: f.namespaceID, ServiceKeyID: f.serviceKeyID,
			Operations: []corev2.Operation{corev2.OperationInvoke}, AllowedAgentIDs: []corev2.Ref{f.agentID}, CanAssertScope: true,
		},
		Agent: "assistant", Endpoint: provider.EndpointChatCompletions,
		Scope:         corev2.Scope{Tenant: "clinic/a", BillTo: "clinic/a", Feature: "assistant"},
		InvocationKey: key, RequestHash: sha256.Sum256([]byte(key)), RequestedModel: "gpt-safe",
		InputUpperBound: 100, InputBounded: true, OutputUpperBound: 5, OutputBounded: true,
	}
}

func seedProviderFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cipher v2postgres.SecretCipher) providerFixture {
	t.Helper()
	runner, err := v2postgres.NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	accountID, namespaceID := corev2.Ref(ids.New("acc")), corev2.Ref(ids.New("nsp"))
	serviceKeyID, agentID := corev2.Ref(ids.New("key")), corev2.Ref(ids.New("agt"))
	secretID, routeID := ids.New("sec"), ids.New("rte")
	scope := v2postgres.Scope{AccountID: string(accountID), NamespaceID: string(namespaceID)}
	sealed, keyID, err := cipher.Seal(ctx, v2postgres.SecretAAD{AccountID: string(accountID), NamespaceID: string(namespaceID), Name: "openai", Version: 1}, []byte("provider-secret"))
	if err != nil {
		t.Fatal(err)
	}
	verifier := sha256.Sum256([]byte("fixture-service-key"))
	err = runner.WithScope(ctx, scope, func(ctx context.Context, db v2postgres.DBTX) error {
		if _, err := db.Exec(ctx, `INSERT INTO kave_v2.namespaces (id, account_id, application, environment) VALUES ($1,$2,'gateway-test',$3)`, namespaceID, accountID, ids.New("env")); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.secrets (id, account_id, namespace_id, name, backend, ciphertext, wrapping_key_id, version)
VALUES ($1,$2,$3,'openai','encrypted',$4,$5,1)
`, secretID, accountID, namespaceID, sealed, keyID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.provider_routes (
  id, account_id, namespace_id, name, provider, base_url, secret_id, model_policy, pricing_revision, pricing
) VALUES ($1,$2,$3,'openai','openai','https://api.openai.com/v1',$4,
  '{"allowed_models":["gpt-safe"],"default_model":"gpt-safe"}', 7,
  '{"models":{"gpt-safe":{"input_nanos_per_million_tokens":2000000,"output_nanos_per_million_tokens":4000000}}}')
`, routeID, accountID, namespaceID, secretID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `INSERT INTO kave_v2.agents (id,account_id,namespace_id,name,kind,route_id) VALUES ($1,$2,$3,'assistant','llm',$4)`, agentID, accountID, namespaceID, routeID); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys (
  id,account_id,namespace_id,name,lookup_prefix,secret_hash,capabilities,allowed_agent_ids,can_assert_scope
) VALUES ($1,$2,$3,'worker',$4,$5,ARRAY['invoke'],ARRAY[$6],TRUE)
`, serviceKeyID, accountID, namespaceID, "lookup_"+ids.New("pfx"), verifier[:], agentID); err != nil {
			return err
		}
		for _, limit := range []struct {
			id, key, metric string
			cap             int64
		}{
			{ids.New("lim"), "requests", "requests", 2}, {ids.New("lim"), "input", "input_tokens", 1000},
			{ids.New("lim"), "output", "output_tokens", 50}, {ids.New("lim"), "cost", "cost_nano_usd", 5000},
		} {
			currency := any(nil)
			if limit.metric == "cost_nano_usd" {
				currency = "USD"
			}
			if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.limits (id,account_id,namespace_id,external_key,metric,currency,tenant_ref,agent_id,hard_cap,window_kind)
VALUES ($1,$2,$3,$4,$5,$6,'clinic/a',$7,$8,'calendar_month')
`, limit.id, accountID, namespaceID, limit.key, limit.metric, currency, agentID, limit.cap); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return providerFixture{runner: runner, scope: scope, accountID: accountID, namespaceID: namespaceID, serviceKeyID: serviceKeyID, agentID: agentID}
}
