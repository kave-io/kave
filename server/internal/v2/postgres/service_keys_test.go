package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestServiceKeyAuthenticatorUsesPrefixOnlyLookupAndVerifiesRawToken(t *testing.T) {
	t.Parallel()

	const (
		prefix = "A1b2C3d4E5f6G7h8I9j0K1l2"
		rawKey = "kv2_" + prefix + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	)
	digest := sha256.Sum256([]byte(rawKey))
	expiresAt := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	authenticator := &ServiceKeyAuthenticator{
		now: func() time.Time { return expiresAt.Add(-time.Hour) },
		queryRow: func(_ context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "FROM kave_v2.lookup_service_key($1)") {
				t.Fatalf("authentication bypasses restricted lookup function: %s", sql)
			}
			if strings.Contains(sql, "FROM kave_v2.service_keys") {
				t.Fatalf("authentication reads service_keys directly: %s", sql)
			}
			if len(args) != 1 || args[0] != prefix {
				t.Fatalf("database args = %#v, want prefix only", args)
			}
			return serviceKeyRow(digest[:], "active", &expiresAt)
		},
	}

	identity, err := authenticator.Authenticate(context.Background(), prefix, rawKey)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if identity.AccountID != "account/acme" || identity.NamespaceID != "namespace/prod" || identity.ServiceKeyID != "key/worker" {
		t.Fatalf("identity = %+v", identity)
	}
	if !identity.CanAssertScope || strings.Join(identity.Capabilities, ",") != "consume,invoke" || strings.Join(identity.AllowedAgentIDs, ",") != "agent/assistant" {
		t.Fatalf("capabilities = %+v", identity)
	}
}

func TestServiceKeyAuthenticatorRejectsInvalidCredentials(t *testing.T) {
	t.Parallel()

	const (
		prefix = "A1b2C3d4E5f6G7h8I9j0K1l2"
		rawKey = "kv2_" + prefix + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	)
	digest := sha256.Sum256([]byte(rawKey))
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		hash      []byte
		status    string
		expiresAt *time.Time
		candidate string
	}{
		{name: "wrong raw token", hash: digest[:], status: "active", candidate: rawKey + "wrong"},
		{name: "malformed verifier", hash: []byte("short"), status: "active", candidate: rawKey},
		{name: "revoked", hash: digest[:], status: "revoked", candidate: rawKey},
		{name: "expired", hash: digest[:], status: "active", expiresAt: &now, candidate: rawKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			authenticator := &ServiceKeyAuthenticator{
				now:      func() time.Time { return now },
				queryRow: func(context.Context, string, ...any) pgx.Row { return serviceKeyRow(tt.hash, tt.status, tt.expiresAt) },
			}
			_, err := authenticator.Authenticate(context.Background(), prefix, tt.candidate)
			if !errors.Is(err, ErrInvalidServiceKey) {
				t.Fatalf("Authenticate() error = %v, want ErrInvalidServiceKey", err)
			}
		})
	}
}

func TestServiceKeyAuthenticatorRejectsBadPrefixBeforeDatabase(t *testing.T) {
	t.Parallel()

	queries := 0
	authenticator := &ServiceKeyAuthenticator{queryRow: func(context.Context, string, ...any) pgx.Row {
		queries++
		return scanRow(func(...any) error { return errors.New("unexpected query") })
	}}
	_, err := authenticator.Authenticate(context.Background(), "too-short", "raw")
	if !errors.Is(err, ErrInvalidServiceKey) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidServiceKey", err)
	}
	if queries != 0 {
		t.Fatalf("database queried %d times for invalid prefix", queries)
	}
}

func TestServiceKeyAuthenticatorMasksMissingKey(t *testing.T) {
	t.Parallel()

	authenticator := &ServiceKeyAuthenticator{queryRow: func(context.Context, string, ...any) pgx.Row {
		return scanRow(func(...any) error { return pgx.ErrNoRows })
	}}
	_, err := authenticator.Authenticate(context.Background(), "A1b2C3d4E5f6G7h8I9j0K1l2", "kv2_A1b2C3d4E5f6G7h8I9j0K1l2.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if !errors.Is(err, ErrInvalidServiceKey) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidServiceKey", err)
	}
}

func TestParseAndAuthenticateRawServiceKey(t *testing.T) {
	t.Parallel()

	const (
		prefix = "A1b2C3d4E5f6G7h8I9j0K1l2"
		rawKey = "kv2_" + prefix + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	)
	digest := sha256.Sum256([]byte(rawKey))
	authenticator := &ServiceKeyAuthenticator{
		queryRow: func(_ context.Context, _ string, args ...any) pgx.Row {
			if len(args) != 1 || args[0] != prefix {
				t.Fatalf("lookup args = %#v, want prefix", args)
			}
			return serviceKeyRow(digest[:], "active", nil)
		},
	}
	identity, err := authenticator.AuthenticateRaw(context.Background(), rawKey)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ServiceKeyID != "key/worker" {
		t.Fatalf("identity = %+v", identity)
	}

	for _, malformed := range []string{
		"", "kv2_short.secret", "kv2_" + prefix + ".short", "other_" + prefix + ".abcdefghijklmnopqrstuvwxyz0123456789",
		rawKey + "\r\nX-Evil: true",
	} {
		if _, err := ParseServiceKey(malformed); !errors.Is(err, ErrInvalidServiceKey) {
			t.Fatalf("ParseServiceKey(%q) error = %v, want invalid", malformed, err)
		}
	}
}

func serviceKeyRow(hash []byte, status string, expiresAt *time.Time) pgx.Row {
	return scanRow(func(dest ...any) error {
		*(dest[0].(*string)) = "account/acme"
		*(dest[1].(*string)) = "namespace/prod"
		*(dest[2].(*string)) = "key/worker"
		*(dest[3].(*[]byte)) = append([]byte(nil), hash...)
		*(dest[4].(*[]string)) = []string{"consume", "invoke"}
		*(dest[5].(*[]string)) = []string{"agent/assistant"}
		*(dest[6].(*bool)) = true
		*(dest[7].(*string)) = status
		*(dest[8].(**time.Time)) = expiresAt
		return nil
	})
}
