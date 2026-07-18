package postgres_test

import (
	"bytes"
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
)

func TestSecretStorePostgres_EncryptedReplayRotateAndRevoke(t *testing.T) {
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
	migrator, err := v2postgres.NewMigrator(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	accountID := corev2.Ref(ids.New("acc"))
	namespaceID := corev2.Ref(ids.New("nsp"))
	adminKeyID := corev2.Ref(ids.New("key"))
	scope := v2postgres.Scope{AccountID: string(accountID), NamespaceID: string(namespaceID)}
	runner, err := v2postgres.NewScopedRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	verifier := sha256.Sum256([]byte("test-admin-key"))
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db v2postgres.DBTX) error {
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.namespaces (id, account_id, application, environment)
VALUES ($1, $2, 'secret-test', $3)
`, namespaceID, accountID, ids.New("env")); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys (
    id, account_id, namespace_id, name, lookup_prefix, secret_hash, capabilities
) VALUES ($1, $2, $3, 'admin', $4, $5, ARRAY['secrets.write'])
`, adminKeyID, accountID, namespaceID, ids.New("pfx"), verifier[:])
		return err
	}); err != nil {
		t.Fatal(err)
	}

	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(i + 1)
	}
	cipher, err := v2postgres.NewLocalEnvelopeCipher(hex.EncodeToString(master))
	if err != nil {
		t.Fatal(err)
	}
	store, err := v2postgres.NewSecretStore(pool, cipher)
	if err != nil {
		t.Fatal(err)
	}
	admin := corev2.Caller{
		AccountID: accountID, NamespaceID: namespaceID, ServiceKeyID: adminKeyID,
		Operations: []corev2.Operation{corev2.OperationSecretsWrite},
	}
	request := corev2.PutSecretRequest{
		Caller: admin, NamespaceID: namespaceID, Name: "openai",
		Plaintext: []byte("provider-secret-one"), IdempotencyKey: "secret/one",
	}
	first, err := store.PutSecret(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	rotatedCallerRequest := request
	rotatedCallerRequest.Caller = admin
	replay, err := store.PutSecret(ctx, rotatedCallerRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.ID != first.ID || replay.Version != 1 {
		t.Fatalf("replay = %+v, first = %+v", replay, first)
	}
	conflict := request
	conflict.Plaintext = []byte("different")
	if _, err := store.PutSecret(ctx, conflict); !errors.Is(err, corev2.ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
	request.Plaintext = []byte("provider-secret-two")
	request.IdempotencyKey = "secret/two"
	rotated, err := store.PutSecret(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ID != first.ID || rotated.Version != 2 {
		t.Fatalf("rotated = %+v", rotated)
	}

	var ciphertext []byte
	var wrappingKeyID, name, status string
	var version int64
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db v2postgres.DBTX) error {
		return db.QueryRow(ctx, `
SELECT ciphertext, wrapping_key_id, name, version, status
FROM kave_v2.secrets WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, accountID, namespaceID, first.ID).Scan(&ciphertext, &wrappingKeyID, &name, &version, &status)
	}); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("provider-secret")) || version != 2 || status != "active" {
		t.Fatalf("unsafe or incorrect persisted secret: version=%d status=%s", version, status)
	}
	plaintext, err := cipher.Open(ctx, v2postgres.SecretAAD{
		AccountID: string(accountID), NamespaceID: string(namespaceID), Name: name, Version: version,
	}, ciphertext, wrappingKeyID)
	if err != nil || string(plaintext) != "provider-secret-two" {
		t.Fatalf("decrypt = %q, %v", plaintext, err)
	}
	clear(plaintext)

	concurrent := request
	concurrent.Caller = admin
	concurrent.Plaintext = []byte("provider-secret-three")
	concurrent.IdempotencyKey = "secret/concurrent"
	type putOutcome struct {
		metadata corev2.SecretMetadata
		err      error
	}
	outcomes := make(chan putOutcome, 8)
	for range 8 {
		go func() {
			metadata, putErr := store.PutSecret(ctx, concurrent)
			outcomes <- putOutcome{metadata: metadata, err: putErr}
		}()
	}
	createdCount, replayCount := 0, 0
	for range 8 {
		outcome := <-outcomes
		if outcome.err != nil {
			t.Fatalf("concurrent PutSecret: %v", outcome.err)
		}
		if outcome.metadata.Version != 3 || outcome.metadata.ID != first.ID {
			t.Fatalf("concurrent metadata = %+v", outcome.metadata)
		}
		if outcome.metadata.Replayed {
			replayCount++
		} else {
			createdCount++
		}
	}
	if createdCount != 1 || replayCount != 7 {
		t.Fatalf("concurrent create/replay counts = %d/%d", createdCount, replayCount)
	}

	revoke := corev2.RevokeSecretRequest{Caller: admin, ID: corev2.Ref(first.ID), Reason: "rotation test"}
	if err := store.RevokeSecret(ctx, revoke); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeSecret(ctx, revoke); err != nil {
		t.Fatalf("idempotent revoke: %v", err)
	}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db v2postgres.DBTX) error {
		return db.QueryRow(ctx, `SELECT status FROM kave_v2.secrets WHERE id = $1`, first.ID).Scan(&status)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "revoked" {
		t.Fatalf("status = %q", status)
	}
	if err := runner.WithScope(ctx, scope, func(ctx context.Context, db v2postgres.DBTX) error {
		_, err := db.Exec(ctx, `UPDATE kave_v2.namespaces SET status = 'disabled' WHERE id = $1`, namespaceID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	disabledPut := request
	disabledPut.Caller = admin
	disabledPut.IdempotencyKey = "secret/disabled"
	if _, err := store.PutSecret(ctx, disabledPut); !errors.Is(err, corev2.ErrUnauthorized) {
		t.Fatalf("PutSecret in disabled namespace error = %v", err)
	}
	if err := store.RevokeSecret(ctx, revoke); !errors.Is(err, corev2.ErrUnauthorized) {
		t.Fatalf("RevokeSecret in disabled namespace error = %v", err)
	}
}
