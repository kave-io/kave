package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

type adminRow func(...any) error

func (r adminRow) Scan(dest ...any) error { return r(dest...) }

type adminFakeTx struct {
	scope             Scope
	namespaceStatus   string
	agents            map[string]string
	existing          *persistedServiceKey
	storedPrefix      string
	storedHash        []byte
	insertCount       int
	auditDetails      [][]byte
	auditEvents       []string
	auditActors       []string
	auditByID         map[string][]byte
	commitCount       int
	rollbackCount     int
	serviceKeyRevoked bool
	insertErr         error
}

func (t *adminFakeTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	switch {
	case strings.Contains(query, "set_config('kave.account_id'"):
		return adminRow(func(dest ...any) error {
			*(dest[0].(*string)) = t.scope.AccountID
			*(dest[1].(*string)) = t.scope.NamespaceID
			return nil
		})
	case strings.Contains(query, "FROM kave_v2.namespaces"):
		return adminRow(func(dest ...any) error {
			*(dest[0].(*string)) = t.scope.NamespaceID
			*(dest[1].(*string)) = t.namespaceStatus
			return nil
		})
	case strings.Contains(query, "FROM kave_v2.audit_events"):
		raw, ok := t.auditByID[args[2].(string)]
		if !ok {
			return adminRow(func(...any) error { return pgx.ErrNoRows })
		}
		return adminRow(func(dest ...any) error {
			*(dest[0].(*[]byte)) = bytes.Clone(raw)
			return nil
		})
	case strings.Contains(query, "FROM kave_v2.agents"):
		name := args[2].(string)
		agentID, ok := t.agents[name]
		if !ok {
			return adminRow(func(...any) error { return pgx.ErrNoRows })
		}
		return adminRow(func(dest ...any) error {
			*(dest[0].(*string)) = agentID
			*(dest[1].(*string)) = "active"
			return nil
		})
	case strings.Contains(query, "SELECT id, name, lookup_prefix"):
		if t.existing == nil {
			return adminRow(func(...any) error { return pgx.ErrNoRows })
		}
		return adminRow(func(dest ...any) error {
			*(dest[0].(*string)) = t.existing.id
			*(dest[1].(*string)) = t.existing.name
			*(dest[2].(*string)) = t.existing.lookupPrefix
			*(dest[3].(*[]byte)) = bytes.Clone(t.existing.secretHash)
			*(dest[4].(*[]string)) = append([]string(nil), t.existing.operations...)
			*(dest[5].(*[]string)) = append([]string(nil), t.existing.allowedAgentIDs...)
			*(dest[6].(*bool)) = t.existing.canAssertScope
			*(dest[7].(*string)) = t.existing.status
			*(dest[8].(**time.Time)) = cloneTime(t.existing.expiresAt)
			*(dest[9].(*time.Time)) = t.existing.createdAt
			return nil
		})
	case strings.Contains(query, "SELECT name, status"):
		if t.existing == nil || t.existing.id != string(args[2].(corev2.Ref)) {
			return adminRow(func(...any) error { return pgx.ErrNoRows })
		}
		return adminRow(func(dest ...any) error {
			*(dest[0].(*string)) = t.existing.name
			*(dest[1].(*string)) = t.existing.status
			return nil
		})
	default:
		return adminRow(func(...any) error { return fmt.Errorf("unexpected QueryRow: %s", query) })
	}
}

func (t *adminFakeTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(query, "INSERT INTO kave_v2.service_keys"):
		if t.insertErr != nil {
			return pgconn.CommandTag{}, t.insertErr
		}
		t.insertCount++
		t.storedPrefix = args[4].(string)
		t.storedHash = bytes.Clone(args[5].([]byte))
		t.existing = &persistedServiceKey{
			id:              args[0].(string),
			name:            args[3].(string),
			lookupPrefix:    args[4].(string),
			secretHash:      bytes.Clone(args[5].([]byte)),
			operations:      append([]string(nil), args[6].([]string)...),
			allowedAgentIDs: append([]string(nil), args[7].([]string)...),
			canAssertScope:  args[8].(bool),
			status:          "active",
			expiresAt:       cloneTime(args[9].(*time.Time)),
			createdAt:       args[10].(time.Time),
		}
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	case strings.Contains(query, "UPDATE kave_v2.service_keys"):
		if t.existing == nil || t.existing.status != "active" {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		}
		t.existing.status = "revoked"
		t.serviceKeyRevoked = true
		return pgconn.NewCommandTag("UPDATE 1"), nil
	case strings.Contains(query, "INSERT INTO kave_v2.audit_events"):
		if t.auditByID == nil {
			t.auditByID = make(map[string][]byte)
		}
		if strings.Contains(query, "'service_key.issued'") {
			t.auditEvents = append(t.auditEvents, "service_key.issued")
			t.auditActors = append(t.auditActors, fmt.Sprint(args[3]))
			raw := bytes.Clone(args[6].([]byte))
			t.auditDetails = append(t.auditDetails, raw)
			t.auditByID[args[0].(string)] = raw
		} else {
			t.auditEvents = append(t.auditEvents, args[4].(string))
			t.auditActors = append(t.auditActors, fmt.Sprint(args[3]))
			raw := bytes.Clone(args[6].([]byte))
			t.auditDetails = append(t.auditDetails, raw)
			t.auditByID[args[0].(string)] = raw
		}
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	default:
		return pgconn.CommandTag{}, fmt.Errorf("unexpected Exec: %s", query)
	}
}

func (t *adminFakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected Query")
}

func (t *adminFakeTx) Commit(context.Context) error {
	t.commitCount++
	return nil
}

func (t *adminFakeTx) Rollback(context.Context) error {
	t.rollbackCount++
	return nil
}

func newFakeServiceKeyAdmin(tx *adminFakeTx, now time.Time) *ServiceKeyAdmin {
	return &ServiceKeyAdmin{
		runner: &ScopedRunner{begin: func(context.Context, pgx.TxOptions) (transaction, error) {
			return tx, nil
		}},
		now:   func() time.Time { return now },
		newID: func(prefix string) string { return prefix + "_test" },
	}
}

func TestServiceKeyAdminIssuePersistsClientVerifierAndNeverReceivesRawKey(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	expires := now.Add(24 * time.Hour)
	tx := &adminFakeTx{
		scope:           Scope{AccountID: "account/acme", NamespaceID: "namespace/prod"},
		namespaceStatus: "active",
		agents: map[string]string{
			"clinic-assistant": "agent/clinic",
			"embedder":         "agent/embedder",
		},
	}
	admin := newFakeServiceKeyAdmin(tx, now)
	material := testServiceKeyMaterial(t)
	req := IssueServiceKeyRequest{
		Scope: tx.scope, ActingServiceKeyID: "key/admin", Name: "ai-worker", IdempotencyKey: "issue/ai-worker/1",
		LookupPrefix: material.LookupPrefix, SecretHash: material.SecretHash[:],
		Operations:        []corev2.Operation{corev2.OperationInvoke, corev2.OperationConsume, corev2.OperationConsume},
		AllowedAgentNames: []corev2.Ref{"embedder", "clinic-assistant", "embedder"},
		CanAssertScope:    true,
		ExpiresAt:         &expires,
	}

	first, err := admin.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue() first error = %v", err)
	}
	if !first.Created || first.ID != "key_test" || first.Prefix != RawServiceKeyPrefix+material.LookupPrefix || !first.CreatedAt.Equal(now) {
		t.Fatalf("first issue = %+v", first)
	}
	lookupPrefix, err := ParseServiceKey(material.RawKey)
	if err != nil {
		t.Fatalf("ParseServiceKey(raw) = %v", err)
	}
	if lookupPrefix != tx.storedPrefix {
		t.Fatalf("stored prefix = %q, parsed = %q", tx.storedPrefix, lookupPrefix)
	}
	digest := sha256.Sum256([]byte(material.RawKey))
	if !bytes.Equal(tx.storedHash, digest[:]) {
		t.Fatal("persisted verifier is not SHA-256(raw key)")
	}
	if tx.insertCount != 1 || len(tx.auditEvents) != 1 || tx.auditEvents[0] != "service_key.issued" {
		t.Fatalf("insert/audit = %d/%v", tx.insertCount, tx.auditEvents)
	}
	if tx.auditActors[0] != "key/admin" {
		t.Fatalf("issue audit actor = %q", tx.auditActors[0])
	}
	if bytes.Contains(tx.auditDetails[0], []byte(material.RawKey)) ||
		bytes.Contains(tx.auditDetails[0], []byte(tx.storedPrefix)) ||
		bytes.Contains(tx.auditDetails[0], tx.storedHash) {
		t.Fatalf("audit details contain credential material: %s", tx.auditDetails[0])
	}

	// Natural-key replay never rotates or re-exposes credential material, even
	// after the originally requested expiration has passed or the acting admin
	// credential has rotated.
	admin.now = func() time.Time { return expires.Add(time.Hour) }
	req.ActingServiceKeyID = "key/admin-rotated"
	second, err := admin.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue() replay error = %v", err)
	}
	if second.Created || second.ID != first.ID || second.Prefix != first.Prefix || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("replayed issue exposed material or changed identity: %+v", second)
	}
	if tx.insertCount != 1 || len(tx.auditEvents) != 1 {
		t.Fatalf("replay inserted or audited again: inserts=%d audits=%d", tx.insertCount, len(tx.auditEvents))
	}
	if tx.commitCount != 2 {
		t.Fatalf("commits = %d, want 2", tx.commitCount)
	}
}

func TestServiceKeyAdminIssueRejectsUnsafeOrConflictingSpecifications(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	tx := &adminFakeTx{
		scope:           Scope{AccountID: "account/acme", NamespaceID: "namespace/prod"},
		namespaceStatus: "active",
		agents:          map[string]string{"clinic-assistant": "agent/clinic"},
	}
	admin := newFakeServiceKeyAdmin(tx, now)
	unsafeMaterial := testServiceKeyMaterial(t)

	_, err := admin.Issue(context.Background(), IssueServiceKeyRequest{
		Scope: tx.scope, Name: "unsafe", IdempotencyKey: "issue/unsafe/1",
		LookupPrefix: unsafeMaterial.LookupPrefix, SecretHash: unsafeMaterial.SecretHash[:],
		Operations: []corev2.Operation{corev2.OperationConsume},
	})
	if !errors.Is(err, corev2.ErrInvalidArgument) {
		t.Fatalf("consume without agents error = %v", err)
	}
	if tx.commitCount != 0 {
		t.Fatal("invalid request opened a transaction")
	}

	validMaterial := testServiceKeyMaterial(t)
	valid := IssueServiceKeyRequest{
		Scope: tx.scope, Name: "worker", IdempotencyKey: "issue/worker/1",
		LookupPrefix: validMaterial.LookupPrefix, SecretHash: validMaterial.SecretHash[:],
		Operations:        []corev2.Operation{corev2.OperationConsume},
		AllowedAgentNames: []corev2.Ref{"clinic-assistant"},
	}
	if _, err := admin.Issue(context.Background(), valid); err != nil {
		t.Fatalf("Issue(valid) = %v", err)
	}
	valid.CanAssertScope = true
	if _, err := admin.Issue(context.Background(), valid); !errors.Is(err, corev2.ErrIdempotencyConflict) {
		t.Fatalf("Issue(conflict) error = %v, want idempotency conflict", err)
	}
	valid.CanAssertScope = false
	replacement := testServiceKeyMaterial(t)
	valid.LookupPrefix, valid.SecretHash = replacement.LookupPrefix, replacement.SecretHash[:]
	if _, err := admin.Issue(context.Background(), valid); !errors.Is(err, corev2.ErrIdempotencyConflict) {
		t.Fatalf("Issue(changed material) error = %v, want idempotency conflict", err)
	}
	if tx.insertCount != 1 || len(tx.auditEvents) != 1 {
		t.Fatalf("conflict mutated state: inserts=%d audits=%d", tx.insertCount, len(tx.auditEvents))
	}
}

func TestServiceKeyAdminMapsGlobalLookupCollision(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	tx := &adminFakeTx{
		scope: Scope{AccountID: "account/acme", NamespaceID: "namespace/prod"}, namespaceStatus: "active",
		agents:    map[string]string{},
		insertErr: &pgconn.PgError{Code: "23505", ConstraintName: "service_keys_lookup_prefix_key"},
	}
	material := testServiceKeyMaterial(t)
	_, err := newFakeServiceKeyAdmin(tx, now).Issue(context.Background(), IssueServiceKeyRequest{
		Scope: tx.scope, Name: "worker", IdempotencyKey: "issue/worker/collision",
		LookupPrefix: material.LookupPrefix, SecretHash: material.SecretHash[:],
		Operations: []corev2.Operation{corev2.OperationUsageRead},
	})
	if !errors.Is(err, ErrServiceKeyMaterialConflict) {
		t.Fatalf("Issue(collision) error = %v", err)
	}
	if tx.commitCount != 0 || tx.rollbackCount != maxServiceKeyAttempts {
		t.Fatalf("commit/rollback = %d/%d", tx.commitCount, tx.rollbackCount)
	}
}

func TestServiceKeyAdminRevokeIsScopedAndIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	tx := &adminFakeTx{
		scope:           Scope{AccountID: "account/acme", NamespaceID: "namespace/prod"},
		namespaceStatus: "active",
		agents:          map[string]string{},
		existing: &persistedServiceKey{
			id: "key_worker", name: "worker", operations: []string{"config.apply"}, status: "active",
		},
	}
	admin := newFakeServiceKeyAdmin(tx, now)
	req := RevokeServiceKeyRequest{
		Scope: tx.scope, ActingServiceKeyID: "key_worker", ServiceKeyID: "key_worker", Reason: "rotation",
	}

	first, err := admin.Revoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Revoke() first error = %v", err)
	}
	if !first.Revoked || !tx.serviceKeyRevoked {
		t.Fatalf("first revoke = %+v", first)
	}
	if len(tx.auditEvents) != 1 || tx.auditEvents[0] != "service_key.revoked" {
		t.Fatalf("revoke audits = %v", tx.auditEvents)
	}
	if tx.auditActors[0] != "key_worker" || !bytes.Contains(tx.auditDetails[0], []byte(`"reason":"rotation"`)) {
		t.Fatalf("revoke audit actor/details = %q/%s", tx.auditActors[0], tx.auditDetails[0])
	}

	second, err := admin.Revoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Revoke() replay error = %v", err)
	}
	if second.Revoked {
		t.Fatalf("replayed revoke = %+v, want Revoked=false", second)
	}
	if len(tx.auditEvents) != 1 {
		t.Fatalf("replayed revoke appended audit: %v", tx.auditEvents)
	}

	_, err = admin.Revoke(context.Background(), RevokeServiceKeyRequest{Scope: tx.scope, ServiceKeyID: "key_missing"})
	if !errors.Is(err, ErrServiceKeyNotFound) {
		t.Fatalf("missing revoke error = %v, want ErrServiceKeyNotFound", err)
	}
}

func TestServiceKeyAdminPostgresRoundTrip(t *testing.T) {
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
	migrator, err := NewMigrator(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	scope := Scope{AccountID: ids.New("acc"), NamespaceID: ids.New("nsp")}
	runner, err := NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	routeID, agentID := ids.New("rte"), ids.New("agt")
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.namespaces (id, account_id, application, environment)
VALUES ($1, $2, 'service-key-test', $3)
`, scope.NamespaceID, scope.AccountID, ids.New("env")); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.provider_routes (id, account_id, namespace_id, name, provider, base_url)
VALUES ($1, $2, $3, 'route', 'openai', 'https://api.openai.com')
`, routeID, scope.AccountID, scope.NamespaceID); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.agents (id, account_id, namespace_id, name, kind, route_id)
VALUES ($1, $2, $3, 'clinic-assistant', 'llm', $4)
`, agentID, scope.AccountID, scope.NamespaceID, routeID)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	admin, err := NewServiceKeyAdmin(pool)
	if err != nil {
		t.Fatal(err)
	}
	concurrentMaterial := testServiceKeyMaterial(t)
	concurrentReq := IssueServiceKeyRequest{
		Scope: scope, Name: "concurrent-worker", IdempotencyKey: "issue/concurrent/1",
		LookupPrefix: concurrentMaterial.LookupPrefix, SecretHash: concurrentMaterial.SecretHash[:],
		Operations:        []corev2.Operation{corev2.OperationConsume},
		AllowedAgentNames: []corev2.Ref{"clinic-assistant"}, CanAssertScope: true,
	}
	type issueOutcome struct {
		key IssuedServiceKey
		err error
	}
	outcomes := make(chan issueOutcome, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key, issueErr := admin.Issue(ctx, concurrentReq)
			outcomes <- issueOutcome{key: key, err: issueErr}
		}()
	}
	wg.Wait()
	close(outcomes)
	var concurrentID string
	createdCount := 0
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("concurrent issue: %v", outcome.err)
		}
		if concurrentID == "" {
			concurrentID = outcome.key.ID
		}
		if outcome.key.ID != concurrentID {
			t.Fatalf("concurrent issue IDs differ: %q and %q", concurrentID, outcome.key.ID)
		}
		if outcome.key.Created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent issue created count = %d, want 1", createdCount)
	}

	material := testServiceKeyMaterial(t)
	req := IssueServiceKeyRequest{
		Scope: scope, Name: "worker", IdempotencyKey: "issue/worker/1",
		LookupPrefix: material.LookupPrefix, SecretHash: material.SecretHash[:],
		Operations:        []corev2.Operation{corev2.OperationConsume},
		AllowedAgentNames: []corev2.Ref{"clinic-assistant"}, CanAssertScope: true,
	}
	issued, err := admin.Issue(ctx, req)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !issued.Created || !slicesEqual(issued.AllowedAgentIDs, []string{agentID}) {
		t.Fatalf("issued = %+v", issued)
	}

	authenticator, err := NewServiceKeyAuthenticator(pool)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := authenticator.AuthenticateRaw(ctx, material.RawKey)
	if err != nil {
		t.Fatalf("authenticate issued key: %v", err)
	}
	if identity.AccountID != scope.AccountID || identity.NamespaceID != scope.NamespaceID || identity.ServiceKeyID != issued.ID {
		t.Fatalf("identity = %+v", identity)
	}

	replayed, err := admin.Issue(ctx, req)
	if err != nil {
		t.Fatalf("replay issue: %v", err)
	}
	if replayed.Created || replayed.ID != issued.ID {
		t.Fatalf("replayed = %+v", replayed)
	}

	var storedHash []byte
	var rawInAudit bool
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db DBTX) error {
		if err := db.QueryRow(ctx, `
SELECT secret_hash FROM kave_v2.service_keys
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, scope.AccountID, scope.NamespaceID, issued.ID).Scan(&storedHash); err != nil {
			return err
		}
		return db.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM kave_v2.audit_events
    WHERE account_id = $1 AND namespace_id = $2
      AND details::text LIKE '%' || $3 || '%'
)
`, scope.AccountID, scope.NamespaceID, material.RawKey).Scan(&rawInAudit)
	}); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(material.RawKey))
	if !bytes.Equal(storedHash, digest[:]) || rawInAudit {
		t.Fatalf("stored hash/raw audit invariant failed: hash=%v raw_in_audit=%v", bytes.Equal(storedHash, digest[:]), rawInAudit)
	}

	revoked, err := admin.Revoke(ctx, RevokeServiceKeyRequest{
		Scope: scope, ActingServiceKeyID: corev2.Ref(issued.ID),
		ServiceKeyID: corev2.Ref(issued.ID), Reason: "test rotation",
	})
	if err != nil || !revoked.Revoked {
		t.Fatalf("revoke = %+v, %v", revoked, err)
	}
	if _, err := authenticator.AuthenticateRaw(ctx, material.RawKey); !errors.Is(err, ErrInvalidServiceKey) {
		t.Fatalf("authenticate revoked key error = %v", err)
	}
	secondRevoke, err := admin.Revoke(ctx, RevokeServiceKeyRequest{
		Scope: scope, ActingServiceKeyID: corev2.Ref(issued.ID),
		ServiceKeyID: corev2.Ref(issued.ID), Reason: "test rotation",
	})
	if err != nil || secondRevoke.Revoked {
		t.Fatalf("second revoke = %+v, %v", secondRevoke, err)
	}
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func testServiceKeyMaterial(t *testing.T) corev2.ServiceKeyMaterial {
	t.Helper()
	material, err := corev2.GenerateServiceKeyMaterial(nil)
	if err != nil {
		t.Fatal(err)
	}
	return material
}
