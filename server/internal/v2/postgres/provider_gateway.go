package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/server/internal/v2/provider"
)

const (
	providerLease                            = 15 * time.Minute
	maxProviderAttempts                      = 16
	maxExpiredProviderRecoveriesPerAdmission = 16
	maxProviderCredentialBytes               = 8 * 1024
)

// ProviderStore owns provider admission, conservative reservations, attempts,
// and settlement. Every transaction is RLS-scoped to the authenticated service
// key; prompt and response bodies are never accepted by this type.
type ProviderStore struct {
	runner *ScopedRunner
	cipher SecretCipher
	now    func() time.Time
}

func NewProviderStore(pool *pgxpool.Pool, cipher SecretCipher) (*ProviderStore, error) {
	runner, err := NewScopedRunner(pool)
	if err != nil {
		return nil, err
	}
	return &ProviderStore{runner: runner, cipher: cipher, now: time.Now}, nil
}

type routePriceDocument struct {
	Models map[string]routeModelPrice `json:"models"`
}

type routeModelPrice struct {
	InputNanosPerMillionTokens      int64 `json:"input_nanos_per_million_tokens"`
	OutputNanosPerMillionTokens     int64 `json:"output_nanos_per_million_tokens"`
	CacheReadNanosPerMillionTokens  int64 `json:"cache_read_nanos_per_million_tokens,omitempty"`
	CacheWriteNanosPerMillionTokens int64 `json:"cache_write_nanos_per_million_tokens,omitempty"`
	ReasoningNanosPerMillionTokens  int64 `json:"reasoning_nanos_per_million_tokens,omitempty"`
}

type resolvedProvider struct {
	agentID, agentKind, routeID, provider, protocol, baseURL string
	modelPolicy                                              routeModelPolicy
	validations                                              routeValidationDocument
	pricingRevision                                          int64
	pricing                                                  routePriceDocument
	secretName, secretBackend, wrappingKeyID                 string
	secretVersion                                            int64
	ciphertext                                               []byte
}

type providerLimit struct {
	applicableLimit
	metric   corev2.Metric
	currency string
	reserve  int64
}

type priorProviderInvocation struct {
	id             string
	requestHash    []byte
	status         string
	leaseExpiresAt *time.Time
	agentID        string
	routeID        string
	model          string
}

func (s *ProviderStore) Begin(ctx context.Context, req provider.BeginRequest) (provider.Grant, error) {
	if err := validateProviderBegin(req); err != nil {
		return provider.Grant{}, err
	}
	for attempt := 1; attempt <= maxAdmissionAttempts; attempt++ {
		var grant provider.Grant
		var decisionErr error
		err := s.runner.withScope(ctx, Scope{AccountID: string(req.Caller.AccountID), NamespaceID: string(req.Caller.NamespaceID)}, pgx.ReadCommitted,
			func(txCtx context.Context, db DBTX) error {
				var txErr error
				grant, decisionErr, txErr = s.beginTx(txCtx, db, req, s.now().UTC())
				return txErr
			})
		if err == nil {
			return grant, decisionErr
		}
		clear(grant.Credential)
		if !isRetryableTransactionError(err) || attempt == maxAdmissionAttempts {
			return provider.Grant{}, err
		}
	}
	return provider.Grant{}, errors.New("v2 postgres: provider admission retry exhausted")
}

func validateProviderBegin(req provider.BeginRequest) error {
	if err := req.Caller.AccountID.Validate("caller.account_id", true); err != nil {
		return err
	}
	if err := req.Caller.NamespaceID.Validate("caller.namespace_id", true); err != nil {
		return err
	}
	if err := req.Caller.ServiceKeyID.Validate("caller.service_key_id", true); err != nil {
		return err
	}
	if err := req.Agent.ValidateName("agent", true); err != nil {
		return err
	}
	if err := req.InvocationKey.Validate("invocation_key", true); err != nil {
		return err
	}
	if err := req.Scope.ValidateAdmission(); err != nil {
		return err
	}
	if !req.Caller.Allows(corev2.OperationInvoke, req.Agent) || !req.Caller.CanAssertScope {
		return corev2.ErrUnauthorized
	}
	if req.InputUpperBound < 0 || req.OutputUpperBound < 0 {
		return fmt.Errorf("%w: token bounds cannot be negative", corev2.ErrInvalidArgument)
	}
	switch req.Endpoint {
	case provider.EndpointChatCompletions, provider.EndpointResponses, provider.EndpointEmbeddings:
	default:
		return provider.ErrUnsupportedEndpoint
	}
	return nil
}

func (s *ProviderStore) beginTx(ctx context.Context, db DBTX, req provider.BeginRequest, now time.Time) (provider.Grant, error, error) {
	var namespaceStatus string
	if err := db.QueryRow(ctx, `
SELECT status FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
FOR SHARE
`, req.Caller.AccountID, req.Caller.NamespaceID).Scan(&namespaceStatus); err != nil {
		return provider.Grant{}, nil, fmt.Errorf("v2 postgres: lock provider namespace: %w", err)
	}
	if namespaceStatus != "active" {
		return provider.Grant{}, corev2.ErrUnauthorized, nil
	}
	// Reconcile a bounded batch before evaluating this admission. Recovery is
	// intentionally scoped to the authenticated namespace so it needs no
	// cross-tenant database role. This also means an abandoned reservation can
	// no longer block unrelated invocation keys until the original key retries.
	if _, err := reconcileExpiredProviderAttempts(ctx, db, req.Caller, now, maxExpiredProviderRecoveriesPerAdmission); err != nil {
		return provider.Grant{}, nil, err
	}

	resolved, err := loadProviderRoute(ctx, db, req)
	if err != nil {
		return provider.Grant{}, err, nil
	}
	if len(req.Caller.AllowedAgentIDs) == 0 || !slices.Contains(req.Caller.AllowedAgentIDs, corev2.Ref(resolved.agentID)) {
		return provider.Grant{}, corev2.ErrUnauthorized, nil
	}
	if !endpointMatchesKind(req.Endpoint, resolved.agentKind) {
		return provider.Grant{}, provider.ErrUnsupportedEndpoint, nil
	}
	model, err := resolveProviderModel(req.RequestedModel, resolved.modelPolicy)
	if err != nil {
		return provider.Grant{}, err, nil
	}
	if !resolved.validations.validates(model, resolved.secretVersion) {
		return provider.Grant{}, provider.ErrRouteUnavailable, nil
	}
	price := priceForModel(resolved.pricingRevision, resolved.pricing, model)
	if price == nil {
		// Cost must never silently become zero because a route was activated
		// without a complete, immutable price snapshot.
		return provider.Grant{}, provider.ErrRouteUnavailable, nil
	}

	var prior priorProviderInvocation
	err = db.QueryRow(ctx, `
SELECT id, request_hash, status, lease_expires_at,
       COALESCE(agent_id, ''), COALESCE(route_id, ''), COALESCE(model, '')
FROM kave_v2.invocations
WHERE account_id = $1 AND namespace_id = $2
  AND operation = 'invoke' AND idempotency_key = $3
FOR UPDATE
`, req.Caller.AccountID, req.Caller.NamespaceID, req.InvocationKey).Scan(
		&prior.id, &prior.requestHash, &prior.status, &prior.leaseExpiresAt,
		&prior.agentID, &prior.routeID, &prior.model,
	)
	invocationID := ids.New("ivk")
	attemptNo := 1
	if err == nil {
		if !bytes.Equal(prior.requestHash, req.RequestHash[:]) || prior.agentID != resolved.agentID || prior.routeID != resolved.routeID || prior.model != model {
			return provider.Grant{}, &corev2.IdempotencyConflictError{Key: req.InvocationKey}, nil
		}
		switch prior.status {
		case "settled", "rejected":
			return provider.Grant{}, fmt.Errorf("%w: %s", provider.ErrAlreadyInvoked, prior.id), nil
		case "pending", "admitted":
			if prior.leaseExpiresAt == nil || prior.leaseExpiresAt.After(now) {
				return provider.Grant{}, fmt.Errorf("%w: %s", provider.ErrInvocationInProgress, prior.id), nil
			}
			if err := recoverExpiredProviderAttempt(ctx, db, req, prior.id, now); err != nil {
				return provider.Grant{}, nil, err
			}
		case "failed", "cancelled":
			// Terminal unsuccessful attempts may be retried under the same
			// logical invocation and idempotency key. Their prior reservations
			// were already settled by Complete.
		default:
			return provider.Grant{}, nil, fmt.Errorf("v2 postgres: invocation %s has invalid status %q", prior.id, prior.status)
		}
		invocationID = prior.id
		attemptNo, err = nextProviderAttempt(ctx, db, req, invocationID)
		if err != nil {
			return provider.Grant{}, nil, err
		}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		if err != nil {
			return provider.Grant{}, nil, fmt.Errorf("v2 postgres: inspect provider idempotency: %w", err)
		}
	}

	if resolved.secretBackend != "encrypted" || s.cipher == nil {
		return provider.Grant{}, provider.ErrRouteUnavailable, nil
	}
	openedCredential, err := s.cipher.Open(ctx, SecretAAD{
		AccountID: string(req.Caller.AccountID), NamespaceID: string(req.Caller.NamespaceID),
		Name: resolved.secretName, Version: resolved.secretVersion,
	}, resolved.ciphertext, resolved.wrappingKeyID)
	if err != nil {
		clear(openedCredential)
		return provider.Grant{}, provider.ErrRouteUnavailable, nil
	}
	credential, validCredential := normalizeProviderCredential(openedCredential)
	clear(openedCredential)
	if !validCredential {
		clear(credential)
		return provider.Grant{}, provider.ErrRouteUnavailable, nil
	}

	if prior.id == "" {
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.invocations (
    id, account_id, namespace_id, service_key_id, agent_id, route_id,
    kind, operation, idempotency_key, request_hash,
    tenant_ref, actor_ref, billing_ref, session_ref, feature, model,
    status, lease_expires_at, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    'provider', 'invoke', $7, $8,
    $9, NULLIF($10, ''), $11, NULLIF($12, ''), NULLIF($13, ''), $14,
    'pending', $15, $16
)
		`, invocationID, req.Caller.AccountID, req.Caller.NamespaceID,
			req.Caller.ServiceKeyID, resolved.agentID, resolved.routeID,
			req.InvocationKey, req.RequestHash[:], req.Scope.Tenant, req.Scope.Actor,
			req.Scope.BillTo, req.Scope.Session, req.Scope.Feature, model,
			now.Add(providerLease), now); err != nil {
			clear(credential)
			return provider.Grant{}, nil, fmt.Errorf("v2 postgres: insert provider invocation: %w", err)
		}
	}

	limits, err := loadProviderLimits(ctx, db, req, resolved.agentID, model, now)
	if err != nil {
		clear(credential)
		return provider.Grant{}, nil, err
	}
	if len(limits) > maxApplicableLimits {
		clear(credential)
		return provider.Grant{}, nil, fmt.Errorf("v2 postgres: %d provider limits exceed safety maximum %d", len(limits), maxApplicableLimits)
	}
	reservationErr := assignProviderReservations(limits, req, price)
	if reservationErr != nil {
		clear(credential)
		if err := rejectProviderInvocation(ctx, db, req, invocationID, attemptNo, now, "reservation_unavailable", reservationErr.Error()); err != nil {
			return provider.Grant{}, nil, err
		}
		return provider.Grant{}, reservationErr, nil
	}

	violations := make([]corev2.Violation, 0)
	for i := range limits {
		limit := &limits[i]
		limit.windowStart, limit.windowEnd, err = admissionWindow(now, limit.applicableLimit)
		if err != nil {
			clear(credential)
			return provider.Grant{}, nil, err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.limit_windows (account_id, namespace_id, limit_id, window_start, window_end)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (account_id, namespace_id, limit_id, window_start) DO NOTHING
`, req.Caller.AccountID, req.Caller.NamespaceID, limit.id, limit.windowStart, limit.windowEnd); err != nil {
			clear(credential)
			return provider.Grant{}, nil, fmt.Errorf("v2 postgres: initialize provider limit %s: %w", limit.key, err)
		}
		if err := db.QueryRow(ctx, `
SELECT used, reserved FROM kave_v2.limit_windows
WHERE account_id = $1 AND namespace_id = $2 AND limit_id = $3 AND window_start = $4
FOR UPDATE
`, req.Caller.AccountID, req.Caller.NamespaceID, limit.id, limit.windowStart).Scan(&limit.used, &limit.reserved); err != nil {
			clear(credential)
			return provider.Grant{}, nil, fmt.Errorf("v2 postgres: lock provider limit %s: %w", limit.key, err)
		}
		current, overflow := addWouldOverflow(limit.used, limit.reserved)
		prospective, exceeds := addWouldOverflow(limit.used, limit.reserved, limit.reserve)
		if exceeds || prospective > limit.hardCap {
			if overflow {
				current = math.MaxInt64
			}
			violations = append(violations, corev2.Violation{
				LimitID: limit.id, LimitKey: corev2.Ref(limit.key), Metric: limit.metric,
				Used: current, Requested: limit.reserve, HardCap: limit.hardCap,
				ResetAt: limit.windowEnd.UnixMilli(),
			})
		}
	}
	if len(violations) > 0 {
		clear(credential)
		decision := corev2.Decision{InvocationID: invocationID, Status: corev2.DecisionRejected, Violations: violations}
		if err := rejectProviderLimit(ctx, db, req, decision, attemptNo, now); err != nil {
			return provider.Grant{}, nil, err
		}
		return provider.Grant{}, &corev2.LimitExceededError{Decision: decision}, nil
	}

	for _, limit := range limits {
		if _, err := db.Exec(ctx, `
UPDATE kave_v2.limit_windows
SET reserved = reserved + $5, revision = revision + 1, updated_at = $6
WHERE account_id = $1 AND namespace_id = $2 AND limit_id = $3 AND window_start = $4
`, req.Caller.AccountID, req.Caller.NamespaceID, limit.id, limit.windowStart, limit.reserve, now); err != nil {
			clear(credential)
			return provider.Grant{}, nil, fmt.Errorf("v2 postgres: reserve provider limit %s: %w", limit.key, err)
		}
		priceJSON := providerPriceJSON(price)
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, limit_id, window_start,
    dedupe_key, event_kind, attempt_no, metric, quantity, price_snapshot, occurred_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'reservation', $8, $9, $10, $11, $12)
`, ids.New("use"), req.Caller.AccountID, req.Caller.NamespaceID, invocationID,
			limit.id, limit.windowStart, fmt.Sprintf("reserve:%s:%d:%s", invocationID, attemptNo, limit.id),
			attemptNo, limit.metric, limit.reserve, priceJSON, now); err != nil {
			clear(credential)
			return provider.Grant{}, nil, fmt.Errorf("v2 postgres: ledger provider reservation %s: %w", limit.key, err)
		}
	}
	decision, _ := json.Marshal(map[string]any{
		"status": "admitted", "reservation_count": len(limits),
		"pricing_revision": resolved.pricingRevision, "attempt_no": attemptNo,
	})
	if _, err := db.Exec(ctx, `
UPDATE kave_v2.invocations
SET status = 'admitted', rejection_code = NULL, admitted_at = $4,
    finished_at = NULL, lease_expires_at = $5, decision = $6
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, req.Caller.AccountID, req.Caller.NamespaceID, invocationID, now, now.Add(providerLease), decision); err != nil {
		clear(credential)
		return provider.Grant{}, nil, fmt.Errorf("v2 postgres: admit provider invocation: %w", err)
	}
	if err := appendProviderAudit(ctx, db, req.Caller, invocationID, now, "gateway.invoke", "allowed", map[string]any{
		"agent_id": resolved.agentID, "route_id": resolved.routeID, "model": model,
	}); err != nil {
		clear(credential)
		return provider.Grant{}, nil, err
	}
	return provider.Grant{
		InvocationID: invocationID, AttemptNo: attemptNo, AccountID: req.Caller.AccountID, NamespaceID: req.Caller.NamespaceID,
		ServiceKeyID: req.Caller.ServiceKeyID, AgentID: resolved.agentID, RouteID: resolved.routeID,
		Provider: resolved.provider, Protocol: resolved.protocol, BaseURL: resolved.baseURL, Model: model,
		Credential: credential, Price: price,
	}, nil, nil
}

func nextProviderAttempt(ctx context.Context, db DBTX, req provider.BeginRequest, invocationID string) (int, error) {
	var previous int
	if err := db.QueryRow(ctx, `
SELECT COALESCE(MAX(attempt_no), 0)
FROM kave_v2.usage_entries
WHERE account_id = $1 AND namespace_id = $2 AND invocation_id = $3
`, req.Caller.AccountID, req.Caller.NamespaceID, invocationID).Scan(&previous); err != nil {
		return 0, fmt.Errorf("v2 postgres: inspect provider attempt sequence: %w", err)
	}
	if previous >= maxProviderAttempts {
		return 0, fmt.Errorf("%w: maximum attempt count reached", provider.ErrReservationUnavailable)
	}
	return previous + 1, nil
}

// recoverExpiredProviderAttempt reconciles an abandoned lease before another
// attempt can reserve budget. If StartAttempt was durably recorded, the prior
// maximum reservation is charged because provider delivery is uncertain. If
// no start exists, egress provably never began and the reservation is released.
func recoverExpiredProviderAttempt(ctx context.Context, db DBTX, req provider.BeginRequest, invocationID string, now time.Time) error {
	var attemptNo int
	var providerName, model string
	if err := db.QueryRow(ctx, `
SELECT COALESCE((invocation.decision->>'attempt_no')::INTEGER, 0),
       COALESCE(route.provider, ''), COALESCE(invocation.model, '')
FROM kave_v2.invocations AS invocation
LEFT JOIN kave_v2.provider_routes AS route
  ON route.account_id = invocation.account_id
 AND route.namespace_id = invocation.namespace_id
 AND route.id = invocation.route_id
WHERE invocation.account_id = $1 AND invocation.namespace_id = $2 AND invocation.id = $3
`, req.Caller.AccountID, req.Caller.NamespaceID, invocationID).Scan(&attemptNo, &providerName, &model); err != nil {
		return fmt.Errorf("v2 postgres: inspect expired provider attempt: %w", err)
	}
	if attemptNo == 0 {
		// Pending records from an older writer may not have an admitted decision.
		// The first provider attempt is the only safe recovery target.
		attemptNo = 1
	}
	var started bool
	if err := db.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM kave_v2.usage_entries
    WHERE account_id = $1 AND namespace_id = $2 AND invocation_id = $3
      AND event_kind = 'provider_attempt' AND attempt_no = $4
      AND attempt_status = 'started'
)
`, req.Caller.AccountID, req.Caller.NamespaceID, invocationID, attemptNo).Scan(&started); err != nil {
		return fmt.Errorf("v2 postgres: inspect expired provider delivery: %w", err)
	}

	rows, err := db.Query(ctx, `
SELECT reservation.limit_id, reservation.window_start, reservation.metric, reservation.quantity
FROM kave_v2.usage_entries AS reservation
WHERE reservation.account_id = $1 AND reservation.namespace_id = $2
  AND reservation.invocation_id = $3 AND reservation.event_kind = 'reservation'
  AND reservation.attempt_no = $4
  AND NOT EXISTS (
      SELECT 1 FROM kave_v2.usage_entries AS settlement
      WHERE settlement.account_id = reservation.account_id
        AND settlement.namespace_id = reservation.namespace_id
        AND settlement.invocation_id = reservation.invocation_id
        AND settlement.event_kind = 'settlement'
        AND settlement.attempt_no = reservation.attempt_no
        AND settlement.limit_id = reservation.limit_id
        AND settlement.window_start = reservation.window_start
  )
ORDER BY reservation.limit_id
`, req.Caller.AccountID, req.Caller.NamespaceID, invocationID, attemptNo)
	if err != nil {
		return fmt.Errorf("v2 postgres: load expired provider reservations: %w", err)
	}
	reservations := make([]reservationRow, 0)
	for rows.Next() {
		var row reservationRow
		if err := rows.Scan(&row.limitID, &row.windowStart, &row.metric, &row.quantity); err != nil {
			rows.Close()
			return fmt.Errorf("v2 postgres: scan expired provider reservation: %w", err)
		}
		reservations = append(reservations, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("v2 postgres: iterate expired provider reservations: %w", err)
	}
	rows.Close()

	accounted := providerUsageAccounting{Estimated: started}
	if started {
		accounted.RequestCount = 1
	}
	for _, reservation := range reservations {
		charged := int64(0)
		if started {
			charged = reservation.quantity
		}
		accounted.include(reservation.metric, charged)
		tag, err := db.Exec(ctx, `
UPDATE kave_v2.limit_windows
SET reserved = reserved - $5, used = used + $6,
    revision = revision + 1, updated_at = $7
WHERE account_id = $1 AND namespace_id = $2
  AND limit_id = $3 AND window_start = $4 AND reserved >= $5
  AND used <= $8
`, req.Caller.AccountID, req.Caller.NamespaceID, reservation.limitID,
			reservation.windowStart, reservation.quantity, charged, now, math.MaxInt64-charged)
		if err != nil || tag.RowsAffected() != 1 {
			if err == nil {
				err = errors.New("expired reservation counter disappeared")
			}
			return fmt.Errorf("v2 postgres: reconcile expired provider reservation: %w", err)
		}
		detail, _ := json.Marshal(map[string]any{"expired_lease": true, "delivery_started": started})
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, limit_id, window_start,
    dedupe_key, event_kind, attempt_no, metric, quantity, usage_detail, occurred_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'settlement', $8, $9, $10, $11, $12)
`, ids.New("use"), req.Caller.AccountID, req.Caller.NamespaceID, invocationID,
			reservation.limitID, reservation.windowStart,
			fmt.Sprintf("settle:%s:%d:%s", invocationID, attemptNo, reservation.limitID),
			attemptNo, reservation.metric, charged, detail, now); err != nil {
			return fmt.Errorf("v2 postgres: ledger expired provider settlement: %w", err)
		}
	}

	detail, _ := json.Marshal(map[string]any{
		"expired_lease": true, "delivery_started": started,
		"accounting_estimate": accounted.Estimated,
	})
	if started {
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, dedupe_key, event_kind,
    attempt_no, provider, model, attempt_status, usage_detail, occurred_at
) SELECT $1, account_id, namespace_id, id, $2, 'provider_attempt',
         $3, $4, model, 'uncertain', $5, $6
FROM kave_v2.invocations
WHERE account_id = $7 AND namespace_id = $8 AND id = $9
ON CONFLICT (account_id, namespace_id, dedupe_key) DO NOTHING
`, ids.New("use"), fmt.Sprintf("attempt:%s:%d:finish", invocationID, attemptNo),
			attemptNo, providerName, detail, now, req.Caller.AccountID,
			req.Caller.NamespaceID, invocationID); err != nil {
			return fmt.Errorf("v2 postgres: ledger expired provider attempt: %w", err)
		}
	}
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, dedupe_key, event_kind,
    attempt_no, request_count, input_tokens, output_tokens, cost_nanos,
    currency, provider, model, usage_detail, occurred_at
) VALUES ($1, $2, $3, $4, $5, 'settlement', $6, $7, $8, $9, $10,
          NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14, $15)
ON CONFLICT (account_id, namespace_id, dedupe_key) DO NOTHING
`, ids.New("use"), req.Caller.AccountID, req.Caller.NamespaceID, invocationID,
		fmt.Sprintf("usage:%s:%d", invocationID, attemptNo), attemptNo,
		accounted.RequestCount, accounted.InputTokens, accounted.OutputTokens,
		accounted.CostNanos, accounted.Currency, providerName, model, detail, now); err != nil {
		return fmt.Errorf("v2 postgres: ledger expired provider usage: %w", err)
	}
	if _, err := db.Exec(ctx, `
UPDATE kave_v2.invocations
SET status = 'failed', finished_at = $4, lease_expires_at = NULL
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, req.Caller.AccountID, req.Caller.NamespaceID, invocationID, now); err != nil {
		return fmt.Errorf("v2 postgres: finish expired provider attempt: %w", err)
	}
	return appendProviderAudit(ctx, db, req.Caller, invocationID, now, "gateway.recover", "succeeded", map[string]any{
		"attempt_no": attemptNo, "delivery_started": started,
	})
}

func loadProviderRoute(ctx context.Context, db DBTX, req provider.BeginRequest) (resolvedProvider, error) {
	var result resolvedProvider
	var policyJSON, pricingJSON, validationJSON []byte
	err := db.QueryRow(ctx, `
SELECT a.id, a.kind, r.id, r.provider, r.protocol, r.base_url,
       r.model_policy, r.pricing_revision, r.pricing,
       s.name, s.backend, COALESCE(s.ciphertext, ''::bytea),
       COALESCE(s.wrapping_key_id, ''), s.version, r.validation_evidence
FROM kave_v2.agents a
JOIN kave_v2.provider_routes r
  ON r.account_id = a.account_id AND r.namespace_id = a.namespace_id AND r.id = a.route_id
JOIN kave_v2.secrets s
  ON s.account_id = r.account_id AND s.namespace_id = r.namespace_id AND s.id = r.secret_id
WHERE a.account_id = $1 AND a.namespace_id = $2 AND a.name = $3
  AND a.status = 'active' AND r.status = 'active' AND s.status = 'active'
  AND r.last_validated_at IS NOT NULL
  AND r.validated_secret_version = s.version
  AND r.validated_model IS NOT NULL
`, req.Caller.AccountID, req.Caller.NamespaceID, req.Agent).Scan(
		&result.agentID, &result.agentKind, &result.routeID, &result.provider,
		&result.protocol, &result.baseURL, &policyJSON, &result.pricingRevision,
		&pricingJSON, &result.secretName, &result.secretBackend, &result.ciphertext,
		&result.wrappingKeyID, &result.secretVersion, &validationJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return resolvedProvider{}, provider.ErrRouteUnavailable
	}
	if err != nil {
		return resolvedProvider{}, fmt.Errorf("v2 postgres: resolve provider route: %w", err)
	}
	if result.protocol != "openai" {
		return resolvedProvider{}, provider.ErrRouteUnavailable
	}
	if err := json.Unmarshal(policyJSON, &result.modelPolicy); err != nil {
		return resolvedProvider{}, provider.ErrRouteUnavailable
	}
	if err := json.Unmarshal(pricingJSON, &result.pricing); err != nil {
		return resolvedProvider{}, provider.ErrRouteUnavailable
	}
	result.validations, err = decodeRouteValidationDocument(validationJSON)
	if err != nil {
		return resolvedProvider{}, provider.ErrRouteUnavailable
	}
	return result, nil
}

func endpointMatchesKind(endpoint, kind string) bool {
	if endpoint == provider.EndpointEmbeddings {
		return kind == string(corev2.AgentEmbedding)
	}
	return kind == string(corev2.AgentLLM)
}

func resolveProviderModel(requested string, policy routeModelPolicy) (string, error) {
	model := requested
	if model == "" {
		model = policy.DefaultModel
	}
	if model == "" || len(model) > 255 {
		return "", fmt.Errorf("%w: route has no valid model", corev2.ErrInvalidArgument)
	}
	if len(policy.AllowedModels) == 0 || !slices.Contains(policy.AllowedModels, model) {
		return "", fmt.Errorf("%w: model is not allowed", corev2.ErrInvalidArgument)
	}
	return model, nil
}

func priceForModel(revision int64, document routePriceDocument, model string) *provider.Price {
	if revision <= 0 {
		return nil
	}
	price, ok := document.Models[model]
	if !ok || price.InputNanosPerMillionTokens < 0 || price.OutputNanosPerMillionTokens < 0 ||
		price.CacheReadNanosPerMillionTokens < 0 || price.CacheWriteNanosPerMillionTokens < 0 ||
		price.ReasoningNanosPerMillionTokens < 0 {
		return nil
	}
	return &provider.Price{
		Revision:                        revision,
		InputNanosPerMillionTokens:      price.InputNanosPerMillionTokens,
		OutputNanosPerMillionTokens:     price.OutputNanosPerMillionTokens,
		CacheReadNanosPerMillionTokens:  price.CacheReadNanosPerMillionTokens,
		CacheWriteNanosPerMillionTokens: price.CacheWriteNanosPerMillionTokens,
		ReasoningNanosPerMillionTokens:  price.ReasoningNanosPerMillionTokens,
	}
}

// normalizeProviderCredential accepts one opaque printable-ASCII bearer token.
// It strips an optional canonical prefix, copies the secret out of the cipher
// buffer, and rejects whitespace/control bytes that could alter HTTP headers.
func normalizeProviderCredential(value []byte) ([]byte, bool) {
	const bearerPrefix = "Bearer "
	if bytes.HasPrefix(value, []byte(bearerPrefix)) {
		value = value[len(bearerPrefix):]
	}
	if len(value) == 0 || len(value) > maxProviderCredentialBytes {
		return nil, false
	}
	for _, b := range value {
		if b < 0x21 || b > 0x7e {
			return nil, false
		}
	}
	return bytes.Clone(value), true
}

func loadProviderLimits(ctx context.Context, db DBTX, req provider.BeginRequest, agentID, model string, now time.Time) ([]providerLimit, error) {
	metrics := []string{string(corev2.MetricRequests), string(corev2.MetricInputTokens), string(corev2.MetricOutputTokens), string(corev2.MetricCostNanoUSD)}
	rows, err := db.Query(ctx, `
SELECT id, external_key, metric, COALESCE(currency, ''), hard_cap, soft_cap,
       window_kind, window_seconds, window_anchor, effective_from, effective_to
FROM kave_v2.limits
WHERE account_id = $1 AND namespace_id = $2 AND enabled
  AND superseded_at IS NULL AND metric = ANY($3::TEXT[])
  AND (tenant_ref IS NULL OR tenant_ref = $4)
  AND (actor_ref IS NULL OR actor_ref = NULLIF($5, ''))
  AND (billing_ref IS NULL OR billing_ref = $6)
  AND (agent_id IS NULL OR agent_id = $7)
  AND (model IS NULL OR model = $8)
  AND (feature IS NULL OR feature = NULLIF($9, ''))
  AND (effective_from IS NULL OR effective_from <= $10)
  AND (effective_to IS NULL OR effective_to > $10)
ORDER BY id
FOR UPDATE
`, req.Caller.AccountID, req.Caller.NamespaceID, metrics, req.Scope.Tenant,
		req.Scope.Actor, req.Scope.BillTo, agentID, model, req.Scope.Feature, now)
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: query provider limits: %w", err)
	}
	defer rows.Close()
	limits := make([]providerLimit, 0)
	for rows.Next() {
		var limit providerLimit
		if err := rows.Scan(&limit.id, &limit.key, &limit.metric, &limit.currency,
			&limit.hardCap, &limit.softCap, &limit.windowKind, &limit.windowSeconds,
			&limit.windowAnchor, &limit.effectiveFrom, &limit.effectiveTo); err != nil {
			return nil, fmt.Errorf("v2 postgres: scan provider limit: %w", err)
		}
		limits = append(limits, limit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("v2 postgres: iterate provider limits: %w", err)
	}
	return limits, nil
}

func assignProviderReservations(limits []providerLimit, req provider.BeginRequest, price *provider.Price) error {
	for i := range limits {
		limit := &limits[i]
		switch limit.metric {
		case corev2.MetricRequests:
			limit.reserve = 1
		case corev2.MetricInputTokens:
			if !req.InputBounded {
				return fmt.Errorf("%w: input token bound required", provider.ErrReservationUnavailable)
			}
			limit.reserve = req.InputUpperBound
		case corev2.MetricOutputTokens:
			if !req.OutputBounded {
				return fmt.Errorf("%w: output token bound required", provider.ErrReservationUnavailable)
			}
			limit.reserve = req.OutputUpperBound
		case corev2.MetricCostNanoUSD:
			if limit.currency != "" && limit.currency != "USD" {
				return fmt.Errorf("%w: cost limit currency must be USD", provider.ErrReservationUnavailable)
			}
			if price == nil || !req.InputBounded || !req.OutputBounded {
				return fmt.Errorf("%w: pricing and token bounds required", provider.ErrReservationUnavailable)
			}
			cost, ok := provider.CalculateMaximumCost(*price, req.InputUpperBound, req.OutputUpperBound)
			if !ok {
				return fmt.Errorf("%w: cost reservation overflow", provider.ErrReservationUnavailable)
			}
			limit.reserve = cost
		default:
			return fmt.Errorf("%w: unsupported provider metric", provider.ErrReservationUnavailable)
		}
	}
	return nil
}

func rejectProviderInvocation(ctx context.Context, db DBTX, req provider.BeginRequest, invocationID string, attemptNo int, now time.Time, code, reason string) error {
	detail, _ := json.Marshal(map[string]any{"status": "rejected", "reason": reason})
	if _, err := db.Exec(ctx, `
UPDATE kave_v2.invocations
SET status = 'rejected', rejection_code = $4, decision = $5, finished_at = $6, lease_expires_at = NULL
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, req.Caller.AccountID, req.Caller.NamespaceID, invocationID, code, detail, now); err != nil {
		return fmt.Errorf("v2 postgres: reject provider invocation: %w", err)
	}
	if _, err := db.Exec(ctx, `
	INSERT INTO kave_v2.usage_entries (id, account_id, namespace_id, invocation_id, dedupe_key, event_kind, attempt_no, usage_detail, occurred_at)
	VALUES ($1, $2, $3, $4, $5, 'block', $6, $7, $8)
`, ids.New("use"), req.Caller.AccountID, req.Caller.NamespaceID, invocationID,
		fmt.Sprintf("block:%s:%d", invocationID, attemptNo), attemptNo, detail, now); err != nil {
		return fmt.Errorf("v2 postgres: ledger provider block: %w", err)
	}
	return appendProviderAudit(ctx, db, req.Caller, invocationID, now, "gateway.invoke", "denied", map[string]any{"code": code})
}

func rejectProviderLimit(ctx context.Context, db DBTX, req provider.BeginRequest, decision corev2.Decision, attemptNo int, now time.Time) error {
	raw, _ := json.Marshal(decision)
	if _, err := db.Exec(ctx, `
UPDATE kave_v2.invocations
SET status = 'rejected', rejection_code = 'limit_exceeded', decision = $4, finished_at = $5, lease_expires_at = NULL
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, req.Caller.AccountID, req.Caller.NamespaceID, decision.InvocationID, raw, now); err != nil {
		return fmt.Errorf("v2 postgres: reject provider limit: %w", err)
	}
	if _, err := db.Exec(ctx, `
	INSERT INTO kave_v2.usage_entries (id, account_id, namespace_id, invocation_id, dedupe_key, event_kind, attempt_no, usage_detail, occurred_at)
	VALUES ($1, $2, $3, $4, $5, 'block', $6, $7, $8)
`, ids.New("use"), req.Caller.AccountID, req.Caller.NamespaceID, decision.InvocationID,
		fmt.Sprintf("block:%s:%d", decision.InvocationID, attemptNo), attemptNo, raw, now); err != nil {
		return fmt.Errorf("v2 postgres: ledger provider limit block: %w", err)
	}
	return appendProviderAudit(ctx, db, req.Caller, decision.InvocationID, now, "gateway.invoke", "denied", map[string]any{"code": "limit_exceeded", "violations": len(decision.Violations)})
}

func (s *ProviderStore) StartAttempt(ctx context.Context, req provider.AttemptRequest) error {
	if req.AttemptNo <= 0 || req.AttemptNo != req.Grant.AttemptNo {
		return fmt.Errorf("%w: attempt number must be positive", corev2.ErrInvalidArgument)
	}
	return s.runner.WithScope(ctx, Scope{AccountID: string(req.Grant.AccountID), NamespaceID: string(req.Grant.NamespaceID)}, func(txCtx context.Context, db DBTX) error {
		var status string
		var currentAttempt int
		if err := db.QueryRow(txCtx, `
SELECT status, COALESCE((decision->>'attempt_no')::INTEGER, 0)
FROM kave_v2.invocations
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
FOR UPDATE
`, req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID).Scan(&status, &currentAttempt); err != nil {
			return fmt.Errorf("v2 postgres: lock provider attempt: %w", err)
		}
		if status != "admitted" || currentAttempt != req.AttemptNo {
			return fmt.Errorf("v2 postgres: invocation is %s, not admitted", status)
		}
		startedAt := req.StartedAt.UTC()
		if startedAt.IsZero() {
			startedAt = s.now().UTC()
		}
		if _, err := db.Exec(txCtx, `
UPDATE kave_v2.invocations
SET lease_expires_at = $4
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID, startedAt.Add(providerLease)); err != nil {
			return fmt.Errorf("v2 postgres: extend provider attempt lease: %w", err)
		}
		detail, _ := json.Marshal(map[string]any{"phase": "started"})
		_, err := db.Exec(txCtx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, dedupe_key, event_kind,
    attempt_no, request_count, provider, model, attempt_status, usage_detail, occurred_at
) VALUES ($1, $2, $3, $4, $5, 'provider_attempt', $6, 1, $7, $8, 'started', $9, $10)
ON CONFLICT (account_id, namespace_id, dedupe_key) DO NOTHING
`, ids.New("use"), req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID,
			fmt.Sprintf("attempt:%s:%d:start", req.Grant.InvocationID, req.AttemptNo), req.AttemptNo,
			req.Grant.Provider, req.Grant.Model, detail, req.StartedAt.UTC())
		if err != nil {
			return fmt.Errorf("v2 postgres: ledger provider attempt start: %w", err)
		}
		return nil
	})
}

// RenewLease keeps a legitimately long streaming call from being mistaken for
// a crashed worker. The attempt number in the invocation decision prevents a
// stale worker from extending a newer retry's lease.
func (s *ProviderStore) RenewLease(ctx context.Context, grant provider.Grant) error {
	if grant.AttemptNo <= 0 {
		return fmt.Errorf("%w: lease attempt is required", corev2.ErrInvalidArgument)
	}
	return s.runner.WithScope(ctx, Scope{AccountID: string(grant.AccountID), NamespaceID: string(grant.NamespaceID)}, func(txCtx context.Context, db DBTX) error {
		tag, err := db.Exec(txCtx, `
UPDATE kave_v2.invocations
SET lease_expires_at = $5
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
  AND status = 'admitted'
  AND COALESCE((decision->>'attempt_no')::INTEGER, 0) = $4
`, grant.AccountID, grant.NamespaceID, grant.InvocationID, grant.AttemptNo, s.now().UTC().Add(providerLease))
		if err != nil {
			return fmt.Errorf("v2 postgres: renew provider lease: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return errors.New("v2 postgres: provider lease is no longer current")
		}
		return nil
	})
}

type reservationRow struct {
	limitID     string
	windowStart time.Time
	metric      corev2.Metric
	quantity    int64
}

type providerUsageAccounting struct {
	RequestCount     int64
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostNanos        int64
	Currency         string
	Estimated        bool
}

func accountingFromCompletion(req provider.CompleteRequest) providerUsageAccounting {
	return providerUsageAccounting{
		RequestCount:     boolToInt64(req.DeliveryStarted),
		InputTokens:      req.Usage.InputTokens,
		OutputTokens:     req.Usage.OutputTokens,
		CacheReadTokens:  req.Usage.CacheReadTokens,
		CacheWriteTokens: req.Usage.CacheWriteTokens,
		ReasoningTokens:  req.Usage.ReasoningTokens,
		CostNanos:        req.Usage.CostNanos,
		Currency:         req.Usage.Currency,
		Estimated:        req.DeliveryStarted && (req.Uncertain || !req.Usage.Reported),
	}
}

func (u *providerUsageAccounting) include(metric corev2.Metric, quantity int64) {
	if u == nil || quantity < 0 {
		return
	}
	var current *int64
	switch metric {
	case corev2.MetricRequests:
		current = &u.RequestCount
	case corev2.MetricInputTokens:
		current = &u.InputTokens
	case corev2.MetricOutputTokens:
		current = &u.OutputTokens
	case corev2.MetricCostNanoUSD:
		current = &u.CostNanos
	default:
		return
	}
	if quantity > *current {
		*current = quantity
		u.Estimated = true
		if metric == corev2.MetricCostNanoUSD && u.Currency == "" {
			u.Currency = "USD"
		}
	}
}

func (s *ProviderStore) Complete(ctx context.Context, req provider.CompleteRequest) error {
	if req.Usage.InputTokens < 0 || req.Usage.OutputTokens < 0 ||
		req.Usage.CacheReadTokens < 0 || req.Usage.CacheWriteTokens < 0 ||
		req.Usage.ReasoningTokens < 0 || req.Usage.CostNanos < 0 {
		return fmt.Errorf("%w: provider usage must not be negative", corev2.ErrInvalidArgument)
	}
	if req.Usage.CacheReadTokens > req.Usage.InputTokens ||
		req.Usage.CacheWriteTokens > req.Usage.InputTokens ||
		req.Usage.ReasoningTokens > req.Usage.OutputTokens {
		return fmt.Errorf("%w: provider usage token details exceed their totals", corev2.ErrInvalidArgument)
	}
	if req.Usage.Currency != "" && req.Usage.Currency != "USD" {
		return fmt.Errorf("%w: provider usage currency must be USD", corev2.ErrInvalidArgument)
	}
	if err := corev2.Ref(req.Usage.Model).Validate("provider_usage.model", false); err != nil {
		return err
	}
	if req.FinishedAt.IsZero() {
		req.FinishedAt = s.now().UTC()
	}
	if req.AttemptNo == 0 {
		req.AttemptNo = req.Grant.AttemptNo
	}
	if req.AttemptNo <= 0 || req.AttemptNo != req.Grant.AttemptNo {
		return fmt.Errorf("%w: completion attempt does not match grant", corev2.ErrInvalidArgument)
	}
	return s.runner.WithScope(ctx, Scope{AccountID: string(req.Grant.AccountID), NamespaceID: string(req.Grant.NamespaceID)}, func(txCtx context.Context, db DBTX) error {
		var status string
		var currentAttempt int
		if err := db.QueryRow(txCtx, `
SELECT status, COALESCE((decision->>'attempt_no')::INTEGER, 0)
FROM kave_v2.invocations
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
FOR UPDATE
`, req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID).Scan(&status, &currentAttempt); err != nil {
			return fmt.Errorf("v2 postgres: lock provider settlement: %w", err)
		}
		var alreadyFinished bool
		if err := db.QueryRow(txCtx, `
SELECT EXISTS (
    SELECT 1 FROM kave_v2.usage_entries
    WHERE account_id = $1 AND namespace_id = $2 AND invocation_id = $3
      AND dedupe_key = $4
)
`, req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID,
			fmt.Sprintf("usage:%s:%d", req.Grant.InvocationID, req.AttemptNo)).Scan(&alreadyFinished); err != nil {
			return fmt.Errorf("v2 postgres: inspect provider settlement replay: %w", err)
		}
		if alreadyFinished {
			return nil
		}
		if err := db.QueryRow(txCtx, `
SELECT EXISTS (
    SELECT 1 FROM kave_v2.usage_entries
    WHERE account_id = $1 AND namespace_id = $2 AND invocation_id = $3
      AND dedupe_key = $4 AND attempt_status = 'started'
)
`, req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID,
			fmt.Sprintf("attempt:%s:%d:start", req.Grant.InvocationID, req.AttemptNo)).Scan(&req.DeliveryStarted); err != nil {
			return fmt.Errorf("v2 postgres: inspect provider delivery start: %w", err)
		}
		if status == "settled" || status == "failed" || status == "cancelled" {
			return nil
		}
		if status != "admitted" || currentAttempt != req.AttemptNo {
			return fmt.Errorf("v2 postgres: invocation is %s, not admitted", status)
		}

		rows, err := db.Query(txCtx, `
SELECT limit_id, window_start, metric, quantity
FROM kave_v2.usage_entries
WHERE account_id = $1 AND namespace_id = $2 AND invocation_id = $3
  AND event_kind = 'reservation' AND attempt_no = $4
  AND NOT EXISTS (
      SELECT 1 FROM kave_v2.usage_entries AS settled
      WHERE settled.account_id = usage_entries.account_id
        AND settled.namespace_id = usage_entries.namespace_id
        AND settled.invocation_id = usage_entries.invocation_id
        AND settled.event_kind = 'settlement'
        AND settled.attempt_no = usage_entries.attempt_no
        AND settled.limit_id = usage_entries.limit_id
        AND settled.window_start = usage_entries.window_start
  )
ORDER BY limit_id
`, req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID, req.AttemptNo)
		if err != nil {
			return fmt.Errorf("v2 postgres: load provider reservations: %w", err)
		}
		reservations := make([]reservationRow, 0)
		for rows.Next() {
			var row reservationRow
			if err := rows.Scan(&row.limitID, &row.windowStart, &row.metric, &row.quantity); err != nil {
				rows.Close()
				return err
			}
			reservations = append(reservations, row)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		sort.Slice(reservations, func(i, j int) bool { return reservations[i].limitID < reservations[j].limitID })

		accounted := accountingFromCompletion(req)
		for _, reservation := range reservations {
			actual := settlementQuantity(req, reservation)
			if actual < 0 {
				return fmt.Errorf("v2 postgres: provider %s usage is invalid", reservation.metric)
			}
			accounted.include(reservation.metric, actual)
			tag, err := db.Exec(txCtx, `
UPDATE kave_v2.limit_windows
SET reserved = reserved - $5, used = used + $6, revision = revision + 1, updated_at = $7
WHERE account_id = $1 AND namespace_id = $2 AND limit_id = $3 AND window_start = $4
  AND reserved >= $5 AND used <= $8
`, req.Grant.AccountID, req.Grant.NamespaceID, reservation.limitID, reservation.windowStart,
				reservation.quantity, actual, req.FinishedAt.UTC(), math.MaxInt64-actual)
			if err != nil || tag.RowsAffected() != 1 {
				if err == nil {
					err = errors.New("reservation counter disappeared")
				}
				return fmt.Errorf("v2 postgres: settle provider reservation: %w", err)
			}
			detail, _ := json.Marshal(map[string]any{"uncertain": req.Uncertain})
			if _, err := db.Exec(txCtx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, limit_id, window_start,
    dedupe_key, event_kind, attempt_no, metric, quantity, usage_detail, occurred_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'settlement', $8, $9, $10, $11, $12)
`, ids.New("use"), req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID,
				reservation.limitID, reservation.windowStart,
				fmt.Sprintf("settle:%s:%d:%s", req.Grant.InvocationID, req.AttemptNo, reservation.limitID),
				req.AttemptNo, reservation.metric, actual, detail, req.FinishedAt.UTC()); err != nil {
				return fmt.Errorf("v2 postgres: ledger provider limit settlement: %w", err)
			}
		}

		attemptStatus := "cancelled"
		finalStatus := "cancelled"
		if req.DeliveryStarted {
			attemptStatus, finalStatus = "failed", "failed"
			if req.HTTPStatus >= 200 && req.HTTPStatus < 400 {
				attemptStatus, finalStatus = "succeeded", "settled"
			}
			if req.Uncertain {
				attemptStatus = "uncertain"
			}
		}
		priceJSON := providerPriceJSON(req.Grant.Price)
		detail, _ := json.Marshal(map[string]any{
			"usage_reported": req.Usage.Reported, "uncertain": req.Uncertain,
			"accounting_estimate": accounted.Estimated,
		})
		var httpStatus any
		if req.HTTPStatus >= 100 && req.HTTPStatus <= 599 {
			httpStatus = req.HTTPStatus
		}
		if req.DeliveryStarted {
			if _, err := db.Exec(txCtx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, dedupe_key, event_kind,
    attempt_no, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
    reasoning_tokens, cost_nanos, currency, provider, model, attempt_status,
    http_status, latency_ms, provider_request_id, price_snapshot, usage_detail, occurred_at
) VALUES (
    $1, $2, $3, $4, $5, 'provider_attempt',
    $6, $7, $8, $9, $10, $11, $12, NULLIF($13, ''), $14, $15, $16,
    $17, $18, NULLIF($19, ''), $20, $21, $22
)
`, ids.New("use"), req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID,
				fmt.Sprintf("attempt:%s:%d:finish", req.Grant.InvocationID, req.AttemptNo), req.AttemptNo,
				req.Usage.InputTokens, req.Usage.OutputTokens, req.Usage.CacheReadTokens,
				req.Usage.CacheWriteTokens, req.Usage.ReasoningTokens, req.Usage.CostNanos,
				req.Usage.Currency, req.Grant.Provider, req.Usage.Model, attemptStatus,
				httpStatus, max(req.Latency.Milliseconds(), 0), req.ProviderRequestID,
				priceJSON, detail, req.FinishedAt.UTC()); err != nil {
				return fmt.Errorf("v2 postgres: ledger provider attempt finish: %w", err)
			}
		}
		usageModel := req.Usage.Model
		if usageModel == "" {
			usageModel = req.Grant.Model
		}
		if _, err := db.Exec(txCtx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, dedupe_key, event_kind,
    attempt_no, request_count,
    input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
    reasoning_tokens, cost_nanos, currency, provider, model, usage_detail, occurred_at
) VALUES ($1, $2, $3, $4, $5, 'settlement', $6, $7, $8, $9, $10, $11, $12, $13, NULLIF($14, ''), $15, $16, $17, $18)
		`, ids.New("use"), req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID,
			fmt.Sprintf("usage:%s:%d", req.Grant.InvocationID, req.AttemptNo), req.AttemptNo,
			accounted.RequestCount,
			accounted.InputTokens, accounted.OutputTokens,
			accounted.CacheReadTokens, accounted.CacheWriteTokens, accounted.ReasoningTokens,
			accounted.CostNanos, accounted.Currency, req.Grant.Provider, usageModel,
			detail, req.FinishedAt.UTC()); err != nil {
			return fmt.Errorf("v2 postgres: ledger provider usage: %w", err)
		}
		if _, err := db.Exec(txCtx, `
UPDATE kave_v2.invocations
SET status = $4, finished_at = $5, lease_expires_at = NULL
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, req.Grant.AccountID, req.Grant.NamespaceID, req.Grant.InvocationID,
			finalStatus, req.FinishedAt.UTC()); err != nil {
			return fmt.Errorf("v2 postgres: finish provider invocation: %w", err)
		}
		caller := corev2.Caller{AccountID: req.Grant.AccountID, NamespaceID: req.Grant.NamespaceID, ServiceKeyID: req.Grant.ServiceKeyID}
		return appendProviderAudit(txCtx, db, caller, req.Grant.InvocationID, req.FinishedAt.UTC(), "gateway.settle", "succeeded", map[string]any{"status": finalStatus, "uncertain": req.Uncertain})
	})
}

func settlementQuantity(req provider.CompleteRequest, reservation reservationRow) int64 {
	var observed int64
	switch reservation.metric {
	case corev2.MetricRequests:
		if req.DeliveryStarted {
			observed = 1
		}
	case corev2.MetricInputTokens:
		observed = req.Usage.InputTokens
	case corev2.MetricOutputTokens:
		observed = req.Usage.OutputTokens
	case corev2.MetricCostNanoUSD:
		observed = req.Usage.CostNanos
	default:
		return -1
	}
	// An uncertain attempt must charge at least the reservation, but never
	// discard a larger amount the provider actually reported.
	if req.Uncertain && reservation.quantity > observed {
		return reservation.quantity
	}
	return observed
}

func boolToInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func providerPriceJSON(price *provider.Price) []byte {
	if price == nil {
		return []byte("{}")
	}
	raw, err := json.Marshal(price)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func appendProviderAudit(ctx context.Context, db DBTX, caller corev2.Caller, invocationID string, now time.Time, event, outcome string, details map[string]any) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("v2 postgres: encode provider audit: %w", err)
	}
	_, err = db.Exec(ctx, `
INSERT INTO kave_v2.audit_events (
    id, account_id, namespace_id, service_key_id, event, resource_type,
    resource_id, outcome, details, created_at
) VALUES ($1, $2, $3, $4, $5, 'invocation', $6, $7, $8, $9)
`, ids.New("aud"), caller.AccountID, caller.NamespaceID, caller.ServiceKeyID,
		event, invocationID, outcome, raw, now)
	if err != nil {
		return fmt.Errorf("v2 postgres: append provider audit: %w", err)
	}
	return nil
}

var _ provider.Store = (*ProviderStore)(nil)
