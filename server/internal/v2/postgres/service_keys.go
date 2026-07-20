package postgres

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"
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

const (
	defaultServiceKeyUsageInterval = 5 * time.Minute
	defaultServiceKeyUsageQueue    = 1024
	defaultServiceKeyUsageKeys     = 8192
	serviceKeyUsageWriteTimeout    = 5 * time.Second
)

// ServiceKeyUsageTrackingOptions bounds last-used telemetry independently of
// authentication. Revocation is still checked by the lookup on every request;
// only the non-security-critical last_used_at write is sampled and moved off
// the request path.
type ServiceKeyUsageTrackingOptions struct {
	Interval  time.Duration
	QueueSize int
	MaxKeys   int
	OnUpdate  func(error)
}

// ServiceKeyAuthenticator performs the only database lookup allowed before
// the account and namespace RLS scope is known. The raw token is compared in
// process and is never sent to Postgres.
type ServiceKeyAuthenticator struct {
	queryRow   queryRowFunc
	recordUsed func(ServiceKeyIdentity, time.Time)
	now        func() time.Time
}

func NewServiceKeyAuthenticator(pool *pgxpool.Pool) (*ServiceKeyAuthenticator, error) {
	return NewServiceKeyAuthenticatorWithUsageTracking(context.Background(), pool, ServiceKeyUsageTrackingOptions{})
}

// NewServiceKeyAuthenticatorWithUsageTracking ties the bounded telemetry
// worker to a serving lifecycle context. Cancel that context only after HTTP
// shutdown has drained active requests.
func NewServiceKeyAuthenticatorWithUsageTracking(ctx context.Context, pool *pgxpool.Pool, options ServiceKeyUsageTrackingOptions) (*ServiceKeyAuthenticator, error) {
	if pool == nil {
		return nil, ErrNilPool
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runner, err := NewScopedRunner(pool)
	if err != nil {
		return nil, err
	}
	markUsed := func(ctx context.Context, identity ServiceKeyIdentity, usedAt time.Time) error {
		return runner.WithScope(ctx, Scope{AccountID: identity.AccountID, NamespaceID: identity.NamespaceID}, func(txCtx context.Context, db DBTX) error {
			tag, err := db.Exec(txCtx, `
UPDATE kave_v2.service_keys AS service_key
SET last_used_at = $4
WHERE service_key.account_id = $1
  AND service_key.namespace_id = $2
  AND service_key.id = $3
  AND service_key.status = 'active'
  AND (service_key.expires_at IS NULL OR service_key.expires_at > $4)
	AND (service_key.last_used_at IS NULL OR service_key.last_used_at <= $4 - INTERVAL '5 minutes')
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
			if tag.RowsAffected() > 1 {
				return errors.New("v2 postgres: service-key usage update affected multiple rows")
			}
			return nil
		})
	}
	tracker := newServiceKeyUsageTracker(ctx, markUsed, options)
	return &ServiceKeyAuthenticator{queryRow: pool.QueryRow, recordUsed: tracker.record, now: time.Now}, nil
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
	if a.recordUsed != nil {
		a.recordUsed(record.ServiceKeyIdentity, now)
	}

	return record.ServiceKeyIdentity, nil
}

type serviceKeyUsageEvent struct {
	identity ServiceKeyIdentity
	usedAt   time.Time
}

type serviceKeyUsageTracker struct {
	mu       sync.Mutex
	next     map[string]time.Time
	events   chan serviceKeyUsageEvent
	interval time.Duration
	maxKeys  int
}

func newServiceKeyUsageTracker(ctx context.Context, mark markServiceKeyUsedFunc, options ServiceKeyUsageTrackingOptions) *serviceKeyUsageTracker {
	interval := options.Interval
	if interval <= 0 {
		interval = defaultServiceKeyUsageInterval
	}
	queueSize := options.QueueSize
	if queueSize <= 0 {
		queueSize = defaultServiceKeyUsageQueue
	}
	maxKeys := options.MaxKeys
	if maxKeys <= 0 {
		maxKeys = defaultServiceKeyUsageKeys
	}
	tracker := &serviceKeyUsageTracker{
		next: make(map[string]time.Time, min(maxKeys, queueSize)), events: make(chan serviceKeyUsageEvent, queueSize),
		interval: interval, maxKeys: maxKeys,
	}
	go tracker.run(ctx, mark, options.OnUpdate)
	return tracker
}

// record is non-blocking. Under overload, authentication succeeds and only a
// sampled telemetry event is dropped. The map and queue are both hard-bounded.
func (t *serviceKeyUsageTracker) record(identity ServiceKeyIdentity, now time.Time) {
	if t == nil || identity.AccountID == "" || identity.NamespaceID == "" || identity.ServiceKeyID == "" {
		return
	}
	key := identity.AccountID + "\x00" + identity.NamespaceID + "\x00" + identity.ServiceKeyID
	t.mu.Lock()
	if next, exists := t.next[key]; exists && next.After(now) {
		t.mu.Unlock()
		return
	}
	if len(t.next) >= t.maxKeys {
		for existing, next := range t.next {
			if !next.After(now) {
				delete(t.next, existing)
			}
		}
	}
	if len(t.next) >= t.maxKeys {
		t.mu.Unlock()
		return
	}
	event := serviceKeyUsageEvent{identity: identity, usedAt: now}
	select {
	case t.events <- event:
		t.next[key] = now.Add(t.interval)
	default:
		// Do not advance next: a later request may enqueue after pressure drops.
	}
	t.mu.Unlock()
}

func (t *serviceKeyUsageTracker) run(ctx context.Context, mark markServiceKeyUsedFunc, onUpdate func(error)) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-t.events:
			writeCtx, cancel := context.WithTimeout(ctx, serviceKeyUsageWriteTimeout)
			err := mark(writeCtx, event.identity, event.usedAt)
			cancel()
			if onUpdate != nil {
				onUpdate(err)
			}
		}
	}
}
