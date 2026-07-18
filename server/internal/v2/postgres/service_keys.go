package postgres

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	corev2 "github.com/kave-io/kave/core/v2"
)

const lookupServiceKeySQL = `
SELECT
    account_id,
    namespace_id,
    service_key_id,
    secret_hash,
    capabilities,
    allowed_agent_ids,
    can_assert_scope,
    status,
    expires_at
FROM kave_v2.lookup_service_key($1)
`

const (
	RawServiceKeyPrefix = corev2.RawServiceKeyPrefix
)

var (
	ErrInvalidServiceKey = errors.New("v2 postgres: invalid service key")
)

// ServiceKeyIdentity is the non-secret identity produced after a raw service
// key has been verified. The persisted verifier is deliberately not exposed.
type ServiceKeyIdentity struct {
	AccountID       string
	NamespaceID     string
	ServiceKeyID    string
	Capabilities    []string
	AllowedAgentIDs []string
	CanAssertScope  bool
}

type serviceKeyRecord struct {
	ServiceKeyIdentity
	secretHash []byte
	status     string
	expiresAt  *time.Time
}

type queryRowFunc func(context.Context, string, ...any) pgx.Row
type markServiceKeyUsedFunc func(context.Context, ServiceKeyIdentity, time.Time) error

// ServiceKeyAuthenticator performs the only database lookup allowed before
// the account and namespace RLS scope is known. The raw token is compared in
// process and is never sent to Postgres.
type ServiceKeyAuthenticator struct {
	queryRow queryRowFunc
	markUsed markServiceKeyUsedFunc
	now      func() time.Time
}

func NewServiceKeyAuthenticator(pool *pgxpool.Pool) (*ServiceKeyAuthenticator, error) {
	if pool == nil {
		return nil, ErrNilPool
	}
	runner, err := NewScopedRunner(pool)
	if err != nil {
		return nil, err
	}
	return &ServiceKeyAuthenticator{
		queryRow: pool.QueryRow,
		markUsed: func(ctx context.Context, identity ServiceKeyIdentity, usedAt time.Time) error {
			return runner.WithScope(ctx, Scope{AccountID: identity.AccountID, NamespaceID: identity.NamespaceID}, func(txCtx context.Context, db DBTX) error {
				tag, err := db.Exec(txCtx, `
UPDATE kave_v2.service_keys AS service_key
SET last_used_at = $4
WHERE service_key.account_id = $1
  AND service_key.namespace_id = $2
  AND service_key.id = $3
  AND service_key.status = 'active'
  AND (service_key.expires_at IS NULL OR service_key.expires_at > $4)
  AND EXISTS (
      SELECT 1 FROM kave_v2.namespaces AS namespace
      WHERE namespace.account_id = service_key.account_id
        AND namespace.id = service_key.namespace_id
        AND namespace.status = 'active'
  )
`, identity.AccountID, identity.NamespaceID, identity.ServiceKeyID, usedAt)
				if err != nil {
					return fmt.Errorf("v2 postgres: mark service key used: %w", err)
				}
				if tag.RowsAffected() != 1 {
					return ErrInvalidServiceKey
				}
				return nil
			})
		},
		now: time.Now,
	}, nil
}

// ParseServiceKey extracts the non-secret lookup prefix from the canonical raw
// key form "kv2_<lookup-prefix>.<secret>". It validates both components before
// any database access while retaining the complete raw key for hash checking.
func ParseServiceKey(rawToken string) (string, error) {
	material, err := corev2.ParseServiceKeyMaterial(rawToken)
	if err != nil {
		return "", ErrInvalidServiceKey
	}
	return material.LookupPrefix, nil
}

// AuthenticateRaw parses and verifies one canonical raw service key.
func (a *ServiceKeyAuthenticator) AuthenticateRaw(ctx context.Context, rawToken string) (ServiceKeyIdentity, error) {
	lookupPrefix, err := ParseServiceKey(rawToken)
	if err != nil {
		return ServiceKeyIdentity{}, err
	}
	return a.Authenticate(ctx, lookupPrefix, rawToken)
}

// Authenticate looks up an intentionally non-secret random prefix through the
// restricted SECURITY DEFINER function, then verifies the complete raw token
// with a constant-time SHA-256 comparison. Invalid, revoked, expired, missing,
// and malformed credentials all produce ErrInvalidServiceKey.
func (a *ServiceKeyAuthenticator) Authenticate(ctx context.Context, lookupPrefix, rawToken string) (ServiceKeyIdentity, error) {
	if a == nil || a.queryRow == nil {
		return ServiceKeyIdentity{}, ErrNilPool
	}
	material, err := corev2.ParseServiceKeyMaterial(rawToken)
	if err != nil || material.LookupPrefix != lookupPrefix {
		return ServiceKeyIdentity{}, ErrInvalidServiceKey
	}

	var record serviceKeyRecord
	err = a.queryRow(ctx, lookupServiceKeySQL, lookupPrefix).Scan(
		&record.AccountID,
		&record.NamespaceID,
		&record.ServiceKeyID,
		&record.secretHash,
		&record.Capabilities,
		&record.AllowedAgentIDs,
		&record.CanAssertScope,
		&record.status,
		&record.expiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ServiceKeyIdentity{}, ErrInvalidServiceKey
	}
	if err != nil {
		return ServiceKeyIdentity{}, fmt.Errorf("v2 postgres: lookup service key: %w", err)
	}

	digest := sha256.Sum256([]byte(rawToken))
	validHash := len(record.secretHash) == sha256.Size && subtle.ConstantTimeCompare(digest[:], record.secretHash) == 1
	now := time.Now().UTC()
	if a.now != nil {
		now = a.now().UTC()
	}
	active := record.status == "active"
	unexpired := record.expiresAt == nil || record.expiresAt.After(now)
	validIdentity := record.AccountID != "" && record.NamespaceID != "" && record.ServiceKeyID != ""
	if !validHash || !active || !unexpired || !validIdentity {
		return ServiceKeyIdentity{}, ErrInvalidServiceKey
	}
	if a.markUsed != nil {
		if err := a.markUsed(ctx, record.ServiceKeyIdentity, now); err != nil {
			return ServiceKeyIdentity{}, err
		}
	}

	return record.ServiceKeyIdentity, nil
}
