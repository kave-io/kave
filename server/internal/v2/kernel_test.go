package v2_test

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
	kernelv2 "github.com/kave-io/kave/proto/gen/kave/kernel/v2"
	"github.com/kave-io/kave/proto/gen/kave/kernel/v2/kernelv2connect"
	v2kernel "github.com/kave-io/kave/server/internal/v2"
	v2postgres "github.com/kave-io/kave/server/internal/v2/postgres"
)

func TestKernelPostgres_OfflineBootstrapThenAuthenticatedControlAndConsume(t *testing.T) {
	dsn := os.Getenv("KAVE_TEST_V2_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_V2_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	migrationPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer migrationPool.Close()

	runtimeRole := "kave_rt_" + strings.ToLower(ids.New("")[:12])
	ownerRole := "kave_owner_" + strings.ToLower(ids.New("")[:12])
	var migrationLogin string
	if err := migrationPool.QueryRow(ctx, "SELECT current_user").Scan(&migrationLogin); err != nil {
		t.Fatal(err)
	}
	if _, err := migrationPool.Exec(ctx, "CREATE ROLE "+pgx.Identifier{ownerRole}.Sanitize()+" NOLOGIN"); err != nil {
		t.Fatal(err)
	}
	if _, err := migrationPool.Exec(ctx, "GRANT "+pgx.Identifier{ownerRole}.Sanitize()+" TO "+pgx.Identifier{migrationLogin}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	var databaseName string
	if err := migrationPool.QueryRow(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if _, err := migrationPool.Exec(ctx, "GRANT CREATE ON DATABASE "+pgx.Identifier{databaseName}.Sanitize()+" TO "+pgx.Identifier{ownerRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if _, err := migrationPool.Exec(ctx, "CREATE ROLE "+pgx.Identifier{runtimeRole}.Sanitize()+" LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		cleanupPool, poolErr := pgxpool.New(cleanupCtx, dsn)
		if poolErr != nil {
			return
		}
		defer cleanupPool.Close()
		role := pgx.Identifier{runtimeRole}.Sanitize()
		_, _ = cleanupPool.Exec(cleanupCtx, "DROP OWNED BY "+role)
		_, _ = cleanupPool.Exec(cleanupCtx, "DROP ROLE "+role)
		owner := pgx.Identifier{ownerRole}.Sanitize()
		_, _ = cleanupPool.Exec(cleanupCtx, "REASSIGN OWNED BY "+owner+" TO "+pgx.Identifier{migrationLogin}.Sanitize())
		_, _ = cleanupPool.Exec(cleanupCtx, "DROP OWNED BY "+owner)
		_, _ = cleanupPool.Exec(cleanupCtx, "DROP ROLE "+owner)
	})
	ownerConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	ownerConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{ownerRole}.Sanitize())
		return err
	}
	ownerPool, err := pgxpool.NewWithConfig(ctx, ownerConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer ownerPool.Close()
	if err := v2kernel.Prepare(ctx, ownerPool, runtimeRole); err != nil {
		t.Fatal(err)
	}

	runtimeConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	runtimeConfig.ConnConfig.User = runtimeRole
	runtimeConfig.ConnConfig.Password = ""
	runtimePool, err := pgxpool.NewWithConfig(ctx, runtimeConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimePool.Close()
	if err := runtimePool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	// Bootstrap is an offline database operation, never a persistent HTTP root.
	applyStore, err := v2postgres.NewApplyStore(runtimePool)
	if err != nil {
		t.Fatal(err)
	}
	namespaceName := corev2.Namespace{
		Account:     corev2.Ref("account/acme"),
		Application: "simorq",
		Environment: corev2.Ref("test-" + strings.ToLower(ids.New("")[:12])),
	}
	created, err := corev2.NewApplyService(applyStore).Apply(ctx, corev2.ApplyRequest{
		Caller:   corev2.Caller{AccountID: "account/acme", ServiceKeyID: "offline-bootstrap", Operations: []corev2.Operation{corev2.OperationApply}, Bootstrap: true},
		Manifest: corev2.Manifest{Namespace: namespaceName}, IdempotencyKey: "bootstrap/namespace",
	})
	if err != nil {
		t.Fatal(err)
	}
	keyAdmin, err := v2postgres.NewServiceKeyAdmin(runtimePool)
	if err != nil {
		t.Fatal(err)
	}
	adminMaterial, err := corev2.GenerateServiceKeyMaterial(nil)
	if err != nil {
		t.Fatal(err)
	}
	adminKey, err := keyAdmin.Issue(ctx, v2postgres.IssueServiceKeyRequest{
		Scope:          v2postgres.Scope{AccountID: "account/acme", NamespaceID: string(created.NamespaceID)},
		IdempotencyKey: "bootstrap/admin-key", Name: "control-admin",
		LookupPrefix: adminMaterial.LookupPrefix, SecretHash: adminMaterial.SecretHash[:],
		Operations: []corev2.Operation{
			corev2.OperationConfigApply, corev2.OperationSecretsWrite, corev2.OperationKeysManage,
			corev2.OperationLimitsSync, corev2.OperationUsageRead, corev2.OperationAuditRead,
		},
	})
	if err != nil || adminKey.ID == "" {
		t.Fatalf("offline admin key = %+v, %v", adminKey, err)
	}

	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(i + 1)
	}
	mux := http.NewServeMux()
	if err := v2kernel.Register(ctx, mux, runtimePool, v2kernel.Config{
		MasterKey: hex.EncodeToString(master), RuntimeRole: runtimeRole,
	}); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()
	client := kernelv2connect.NewKernelServiceClient(httpServer.Client(), httpServer.URL)
	call := func(rawKey string) func(*connect.Request[kernelv2.ApplyRequest]) {
		return func(req *connect.Request[kernelv2.ApplyRequest]) { req.Header().Set("Authorization", "Bearer "+rawKey) }
	}

	namespace := &kernelv2.NamespaceSpec{Account: string(namespaceName.Account), Application: string(namespaceName.Application), Environment: string(namespaceName.Environment)}

	put := connect.NewRequest(&kernelv2.PutSecretRequest{
		NamespaceId: string(created.NamespaceID), Name: "openai", IdempotencyKey: "secret/openai/1",
		Value: &kernelv2.PutSecretRequest_Plaintext{Plaintext: []byte("provider-secret")},
	})
	put.Header().Set("Authorization", "Bearer "+adminMaterial.RawKey)
	secret, err := client.PutSecret(ctx, put)
	if err != nil || secret.Msg.GetStatus() != "active" {
		t.Fatalf("PutSecret() = %+v, %v", secret, err)
	}

	fullApply := connect.NewRequest(&kernelv2.ApplyRequest{
		Manifest: &kernelv2.Manifest{
			Namespace: namespace,
			Routes: []*kernelv2.RouteSpec{{
				Name: "openai", Provider: "openai", Secret: "openai", AllowedModels: []string{"gpt-test"}, DefaultModel: "gpt-test",
				PricingRevision: 1, Pricing: []*kernelv2.ModelPrice{{Model: "gpt-test", InputNanosPerMillionTokens: 2_000_000_000, OutputNanosPerMillionTokens: 8_000_000_000}},
			}},
			Agents: []*kernelv2.AgentSpec{{Name: "clinic-assistant", Kind: kernelv2.AgentKind_AGENT_KIND_LLM, Route: "openai", Enabled: true}},
			Limits: []*kernelv2.LimitSpec{{
				Key: "clinic-actions", Metric: "ai_actions", Window: kernelv2.LimitWindow_LIMIT_WINDOW_MONTH,
				HardCap: 1, Enabled: true, Selector: &kernelv2.LimitSelector{Agent: "clinic-assistant", Feature: "ai_actions"},
			}},
		},
		IdempotencyKey: "deploy/full",
	})
	call(adminMaterial.RawKey)(fullApply)
	if _, err := client.Apply(ctx, fullApply); err != nil {
		t.Fatal(err)
	}

	workerMaterial, err := corev2.GenerateServiceKeyMaterial(nil)
	if err != nil {
		t.Fatal(err)
	}
	issue := connect.NewRequest(&kernelv2.IssueServiceKeyRequest{
		NamespaceId: string(created.NamespaceID), Name: "ai-worker", Operations: []string{"consume"},
		AllowedAgents: []string{"clinic-assistant"}, CanAssertScope: true, IdempotencyKey: "key/worker/1",
		LookupPrefix: workerMaterial.LookupPrefix, SecretHash: workerMaterial.SecretHash[:],
	})
	issue.Header().Set("Authorization", "Bearer "+adminMaterial.RawKey)
	issued, err := client.IssueServiceKey(ctx, issue)
	if err != nil || issued.Msg.GetPrefix() != corev2.RawServiceKeyPrefix+workerMaterial.LookupPrefix {
		t.Fatalf("IssueServiceKey() = %+v, %v", issued, err)
	}

	consume := func(key string) (*connect.Response[kernelv2.ConsumeResponse], error) {
		req := connect.NewRequest(&kernelv2.ConsumeRequest{
			Agent: "clinic-assistant", Metric: "ai_actions", Units: 1, IdempotencyKey: key,
			Scope: &kernelv2.Scope{Tenant: "clinic/a", BillTo: "clinic/a", Feature: "ai_actions"},
		})
		req.Header().Set("Authorization", "Bearer "+workerMaterial.RawKey)
		return client.Consume(ctx, req)
	}
	first, err := consume("run/1")
	if err != nil || first.Msg.GetStatus() != kernelv2.DecisionStatus_DECISION_STATUS_ADMITTED {
		t.Fatalf("first Consume() = %+v, %v", first, err)
	}
	replay, err := consume("run/1")
	if err != nil || !replay.Msg.GetReplayed() || replay.Msg.GetInvocationId() != first.Msg.GetInvocationId() {
		t.Fatalf("replay Consume() = %+v, %v", replay, err)
	}
	if _, err := consume("run/2"); connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("over-cap Consume code = %v, err=%v", connect.CodeOf(err), err)
	}

	runner, err := v2postgres.NewScopedRunner(runtimePool)
	if err != nil {
		t.Fatal(err)
	}
	var plaintextMatches int
	var inputPrice int64
	if err := runner.WithScope(ctx, v2postgres.Scope{AccountID: "account/acme", NamespaceID: string(created.NamespaceID)}, func(ctx context.Context, db v2postgres.DBTX) error {
		if err := db.QueryRow(ctx, `
SELECT count(*) FROM kave_v2.secrets
WHERE id = $1 AND position($2::BYTEA IN ciphertext) > 0
`, secret.Msg.GetId(), []byte("provider-secret")).Scan(&plaintextMatches); err != nil {
			return err
		}
		return db.QueryRow(ctx, `
SELECT (pricing->'models'->'gpt-test'->>'input_nanos_per_million_tokens')::BIGINT
FROM kave_v2.provider_routes
WHERE account_id = $1 AND namespace_id = $2 AND name = 'openai'
`, "account/acme", created.NamespaceID).Scan(&inputPrice)
	}); err != nil {
		t.Fatal(err)
	}
	if plaintextMatches != 0 {
		t.Fatalf("provider plaintext persisted: %d matches", plaintextMatches)
	}
	if inputPrice != 2_000_000_000 {
		t.Fatalf("declarative input price = %d", inputPrice)
	}
}
