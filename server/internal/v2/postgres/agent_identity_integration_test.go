package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

func TestApplyStorePostgres_ReaddingPrunedAgentCreatesNewIdentity(t *testing.T) {
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

	applyStore, err := NewApplyStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	account := corev2.Ref(ids.New("acc"))
	namespace := corev2.Namespace{
		Account: account, Application: corev2.Ref("agent-identity-" + ids.New("app")), Environment: "test",
	}
	bootstrap := corev2.Caller{
		AccountID: account, ServiceKeyID: "offline-bootstrap",
		Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true,
	}
	created, err := applyStore.Apply(ctx, corev2.ApplyRequest{
		Caller: bootstrap, Manifest: corev2.Manifest{Namespace: namespace},
		IdempotencyKey: "agent-identity/bootstrap",
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{AccountID: string(account), NamespaceID: string(created.NamespaceID)}
	runner, err := NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.secrets (
    id, account_id, namespace_id, name, backend, ciphertext, wrapping_key_id
) VALUES ($1, $2, $3, 'provider', 'encrypted', $4, 'test-key')
`, ids.New("sec"), account, created.NamespaceID, []byte("test-ciphertext"))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	manifest := corev2.Manifest{
		Namespace: namespace,
		Routes: []corev2.RouteSpec{{
			Name: "route", Provider: "openai", Secret: "provider",
			AllowedModels: []string{"test-model"}, DefaultModel: "test-model", PricingRevision: 1,
			Pricing: []corev2.ModelPrice{{Model: "test-model"}},
		}},
		Agents: []corev2.AgentSpec{{
			Name: "assistant", Kind: corev2.AgentLLM, Route: "route", Enabled: true,
		}},
	}
	configured, err := applyStore.Apply(ctx, corev2.ApplyRequest{
		Caller: bootstrap, Manifest: manifest, ExpectedRevision: created.Revision,
		IdempotencyKey: "agent-identity/configure",
	})
	if err != nil {
		t.Fatal(err)
	}
	oldAgentID := currentAgentID(t, ctx, runner, scope, "assistant")

	keyAdmin, err := NewServiceKeyAdmin(pool)
	if err != nil {
		t.Fatal(err)
	}
	oldKey := issueAgentIdentityTestKey(t, ctx, keyAdmin, scope, "old-worker", "old", "assistant")
	if len(oldKey.AllowedAgentIDs) != 1 || oldKey.AllowedAgentIDs[0] != oldAgentID {
		t.Fatalf("old key allowed agents = %v, want [%s]", oldKey.AllowedAgentIDs, oldAgentID)
	}

	pruned, err := applyStore.Apply(ctx, corev2.ApplyRequest{
		Caller: bootstrap, Manifest: corev2.Manifest{Namespace: namespace}, Prune: true,
		ExpectedRevision: configured.Revision, IdempotencyKey: "agent-identity/prune",
	})
	if err != nil {
		t.Fatal(err)
	}
	resurrectionErr := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		_, err := db.Exec(ctx, `
UPDATE kave_v2.agents
SET status = 'active'
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, account, created.NamespaceID, oldAgentID)
		return err
	})
	var immutableErr *pgconn.PgError
	if !errors.As(resurrectionErr, &immutableErr) || immutableErr.Code != "55000" {
		t.Fatalf("archived agent resurrection error = %v, want SQLSTATE 55000", resurrectionErr)
	}
	readded, err := applyStore.Apply(ctx, corev2.ApplyRequest{
		Caller: bootstrap, Manifest: manifest, ExpectedRevision: pruned.Revision,
		IdempotencyKey: "agent-identity/readd",
	})
	if err != nil {
		t.Fatal(err)
	}
	if readded.Revision != pruned.Revision+1 {
		t.Fatalf("re-add result = %+v", readded)
	}
	newAgentID := currentAgentID(t, ctx, runner, scope, "assistant")
	if newAgentID == oldAgentID {
		t.Fatalf("re-added agent reused archived identity %q", oldAgentID)
	}
	if err := assertAgentIdentityHistory(ctx, runner, scope, "assistant", oldAgentID, newAgentID); err != nil {
		t.Fatal(err)
	}

	newKey := issueAgentIdentityTestKey(t, ctx, keyAdmin, scope, "new-worker", "new", "assistant")
	if len(newKey.AllowedAgentIDs) != 1 || newKey.AllowedAgentIDs[0] != newAgentID {
		t.Fatalf("new key allowed agents = %v, want [%s]", newKey.AllowedAgentIDs, newAgentID)
	}

	admissionStore, err := NewAdmissionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	oldRequest := agentIdentityConsumeRequest(account, created.NamespaceID, oldKey.ID, oldAgentID, "old-key")
	if _, err := admissionStore.Consume(ctx, oldRequest); !errors.Is(err, corev2.ErrUnauthorized) {
		t.Fatalf("old service key consume error = %v, want unauthorized", err)
	}
	newRequest := agentIdentityConsumeRequest(account, created.NamespaceID, newKey.ID, newAgentID, "new-key")
	decision, err := admissionStore.Consume(ctx, newRequest)
	if err != nil {
		t.Fatalf("new service key consume: %v", err)
	}
	if decision.Status != corev2.DecisionAdmitted {
		t.Fatalf("new service key decision = %+v", decision)
	}
}

func currentAgentID(t *testing.T, ctx context.Context, runner *ScopedRunner, scope Scope, name string) string {
	t.Helper()
	var id string
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		return db.QueryRow(ctx, `
SELECT id FROM kave_v2.agents
WHERE account_id = $1 AND namespace_id = $2 AND name = $3 AND status <> 'archived'
`, scope.AccountID, scope.NamespaceID, name).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func issueAgentIdentityTestKey(
	t *testing.T,
	ctx context.Context,
	admin *ServiceKeyAdmin,
	scope Scope,
	name, suffix string,
	agent corev2.Ref,
) IssuedServiceKey {
	t.Helper()
	material := testServiceKeyMaterial(t)
	issued, err := admin.Issue(ctx, IssueServiceKeyRequest{
		Scope: scope, Name: name, IdempotencyKey: corev2.Ref("agent-identity/key/" + suffix),
		LookupPrefix: material.LookupPrefix, SecretHash: material.SecretHash[:],
		Operations: []corev2.Operation{corev2.OperationConsume}, AllowedAgentNames: []corev2.Ref{agent},
		CanAssertScope: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return issued
}

func assertAgentIdentityHistory(
	ctx context.Context,
	runner *ScopedRunner,
	scope Scope,
	name, oldID, newID string,
) error {
	return runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		rows, err := db.Query(ctx, `
SELECT id, status
FROM kave_v2.agents
WHERE account_id = $1 AND namespace_id = $2 AND name = $3
ORDER BY id
`, scope.AccountID, scope.NamespaceID, name)
		if err != nil {
			return err
		}
		defer rows.Close()
		statuses := map[string]string{}
		for rows.Next() {
			var id, status string
			if err := rows.Scan(&id, &status); err != nil {
				return err
			}
			statuses[id] = status
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(statuses) != 2 || statuses[oldID] != "archived" || statuses[newID] != "active" {
			return fmt.Errorf("agent identity history = %v, want %s:archived and %s:active", statuses, oldID, newID)
		}
		return nil
	})
}

func agentIdentityConsumeRequest(
	accountID, namespaceID corev2.Ref,
	serviceKeyID, agentID string,
	suffix string,
) corev2.ConsumeRequest {
	return corev2.ConsumeRequest{
		Caller: corev2.Caller{
			AccountID: accountID, NamespaceID: namespaceID, ServiceKeyID: corev2.Ref(serviceKeyID),
			Operations:      []corev2.Operation{corev2.OperationConsume},
			AllowedAgentIDs: []corev2.Ref{corev2.Ref(agentID)}, CanAssertScope: true,
		},
		Agent: "assistant", Scope: corev2.Scope{Tenant: "clinic/a", BillTo: "clinic/a", Feature: "ai_actions"},
		Metric: "ai_actions", Units: 1, IdempotencyKey: corev2.Ref("agent-identity/consume/" + suffix),
	}
}
