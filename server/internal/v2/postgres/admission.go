package postgres

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

const (
	maxAdmissionAttempts = 8
	maxApplicableLimits  = 64
)

// AdmissionStore is the Postgres atomicity boundary for exact product quota
// consumption. Provider reservations and settlement build on the same tables
// but remain a separate gateway operation.
type AdmissionStore struct {
	runner *ScopedRunner
	now    func() time.Time
}

func NewAdmissionStore(pool *pgxpool.Pool) (*AdmissionStore, error) {
	runner, err := NewScopedRunner(pool)
	if err != nil {
		return nil, err
	}
	return &AdmissionStore{runner: runner, now: time.Now}, nil
}

func (s *AdmissionStore) Consume(ctx context.Context, req corev2.ConsumeRequest) (corev2.Decision, error) {
	if err := req.Validate(); err != nil {
		return corev2.Decision{}, err
	}
	requestHash, err := req.Hash()
	if err != nil {
		return corev2.Decision{}, err
	}
	hashBytes, err := hex.DecodeString(requestHash)
	if err != nil {
		return corev2.Decision{}, fmt.Errorf("v2 postgres: decode request hash: %w", err)
	}

	for attempt := 1; attempt <= maxAdmissionAttempts; attempt++ {
		var decision corev2.Decision
		var decisionErr error
		// Explicit stable-order row locks provide the admission serialization
		// boundary. Read committed lets a waiter observe the row version produced
		// by the previous holder instead of repeatedly aborting a hot counter via
		// Serializable Snapshot Isolation.
		err = s.runner.withScope(ctx, Scope{
			AccountID:   string(req.Caller.AccountID),
			NamespaceID: string(req.Caller.NamespaceID),
		}, pgx.ReadCommitted, func(txCtx context.Context, db DBTX) error {
			var txErr error
			decision, decisionErr, txErr = s.consumeTx(txCtx, db, req, hashBytes, s.now().UTC())
			return txErr
		})
		if err == nil {
			return decision, decisionErr
		}
		if !isRetryableTransactionError(err) || attempt == maxAdmissionAttempts {
			return corev2.Decision{}, err
		}
	}
	return corev2.Decision{}, err
}

func (s *AdmissionStore) consumeTx(ctx context.Context, db DBTX, req corev2.ConsumeRequest, requestHash []byte, now time.Time) (corev2.Decision, error, error) {
	// Apply and SyncLimits take the same namespace row FOR UPDATE. Holding a
	// shared lock here prevents admission from observing a partially changing
	// set of limit definitions while still permitting unrelated namespaces to
	// proceed concurrently.
	var lockedNamespace, namespaceStatus string
	if err := db.QueryRow(ctx, `
SELECT id, status
FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
FOR SHARE
`, req.Caller.AccountID, req.Caller.NamespaceID).Scan(&lockedNamespace, &namespaceStatus); err != nil {
		return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: lock namespace: %w", err)
	}
	if namespaceStatus != "active" {
		return corev2.Decision{}, nil, fmt.Errorf("%w: namespace is not active", corev2.ErrUnauthorized)
	}

	agentID, err := resolveAgent(ctx, db, req)
	if err != nil {
		return corev2.Decision{}, nil, err
	}
	if len(req.Caller.AllowedAgentIDs) == 0 || !slices.Contains(req.Caller.AllowedAgentIDs, corev2.Ref(agentID)) {
		return corev2.Decision{}, nil, fmt.Errorf("%w: service key cannot consume for agent %q", corev2.ErrUnauthorized, req.Agent)
	}

	// Namespace-scoped idempotency intentionally survives service-key rotation,
	// but replay never bypasses the current caller's operation and agent grant.
	prior, found, err := loadPriorDecision(ctx, db, req, requestHash)
	if err != nil {
		return corev2.Decision{}, nil, err
	}
	if found {
		prior.Replayed = true
		if prior.Status == corev2.DecisionRejected {
			return prior, &corev2.LimitExceededError{Decision: prior}, nil
		}
		return prior, nil, nil
	}

	decision := corev2.Decision{
		InvocationID: ids.New("ivk"),
		Status:       corev2.DecisionAdmitted,
	}
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.invocations (
    id, account_id, namespace_id, service_key_id, agent_id,
    kind, operation, idempotency_key, request_hash,
    tenant_ref, actor_ref, billing_ref, session_ref, feature, model,
    status, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    'consume', 'consume', $6, $7,
    NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''),
    NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''),
    'pending', $14
)
`, decision.InvocationID, req.Caller.AccountID, req.Caller.NamespaceID,
		req.Caller.ServiceKeyID, agentID, req.IdempotencyKey, requestHash,
		req.Scope.Tenant, req.Scope.Actor, req.Scope.BillTo, req.Scope.Session,
		req.Scope.Feature, req.Model, now); err != nil {
		if isUniqueViolation(err) {
			// A SERIALIZABLE retry will observe the concurrently committed
			// idempotency record and return it without consuming again.
			return corev2.Decision{}, nil, err
		}
		return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: insert invocation: %w", err)
	}

	limits, err := loadApplicableLimits(ctx, db, req, agentID, now)
	if err != nil {
		return corev2.Decision{}, nil, err
	}
	if len(limits) > maxApplicableLimits {
		return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: %d applicable limits exceed safety maximum %d", len(limits), maxApplicableLimits)
	}

	for i := range limits {
		limit := &limits[i]
		limit.windowStart, limit.windowEnd, err = admissionWindow(now, *limit)
		if err != nil {
			return corev2.Decision{}, nil, err
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.limit_windows (
    account_id, namespace_id, limit_id, window_start, window_end
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (account_id, namespace_id, limit_id, window_start) DO NOTHING
`, req.Caller.AccountID, req.Caller.NamespaceID, limit.id, limit.windowStart, limit.windowEnd); err != nil {
			return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: initialize limit window %s: %w", limit.key, err)
		}
		if err := db.QueryRow(ctx, `
SELECT used, reserved
FROM kave_v2.limit_windows
WHERE account_id = $1 AND namespace_id = $2
  AND limit_id = $3 AND window_start = $4
FOR UPDATE
`, req.Caller.AccountID, req.Caller.NamespaceID, limit.id, limit.windowStart).
			Scan(&limit.used, &limit.reserved); err != nil {
			return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: lock limit window %s: %w", limit.key, err)
		}

		current, currentOverflow := addWouldOverflow(limit.used, limit.reserved)
		prospective, overflow := addWouldOverflow(limit.used, limit.reserved, req.Units)
		if overflow || prospective > limit.hardCap {
			if currentOverflow {
				current = math.MaxInt64
			}
			decision.Violations = append(decision.Violations, corev2.Violation{
				LimitID:   limit.id,
				LimitKey:  corev2.Ref(limit.key),
				Metric:    req.Metric,
				Used:      current,
				Requested: req.Units,
				HardCap:   limit.hardCap,
				ResetAt:   limit.windowEnd.UnixMilli(),
			})
			continue
		}
		if limit.softCap != nil && prospective > *limit.softCap {
			decision.Warnings = append(decision.Warnings, corev2.Warning{
				LimitID:  limit.id,
				LimitKey: corev2.Ref(limit.key),
				Used:     prospective,
				SoftCap:  *limit.softCap,
				ResetAt:  limit.windowEnd.UnixMilli(),
			})
		}
	}

	if len(decision.Violations) > 0 {
		decision.Status = corev2.DecisionRejected
		for _, limit := range limits {
			if !decisionViolatesLimit(decision, limit.id) {
				continue
			}
			if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, limit_id, window_start,
    dedupe_key, event_kind, metric, quantity, usage_detail, occurred_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'block', $8, $9,
          jsonb_build_object('entry_role', 'limit_window', 'limit_key', $10::text), $11)
`, ids.New("use"), req.Caller.AccountID, req.Caller.NamespaceID,
				decision.InvocationID, limit.id, limit.windowStart,
				limitUsageDedupe("block", decision.InvocationID, limit.id, limit.windowStart),
				req.Metric, req.Units, limit.key, now); err != nil {
				return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: append blocked limit usage %s: %w", limit.key, err)
			}
		}
		if err := finishInvocation(ctx, db, req, decision, now, "rejected", "limit_exceeded"); err != nil {
			return corev2.Decision{}, nil, err
		}
		if err := appendAdmissionAudit(ctx, db, req, decision, now, "denied"); err != nil {
			return corev2.Decision{}, nil, err
		}
		return decision, &corev2.LimitExceededError{Decision: decision}, nil
	}

	for _, limit := range limits {
		tag, err := db.Exec(ctx, `
UPDATE kave_v2.limit_windows
SET used = used + $5, revision = revision + 1, updated_at = $6
WHERE account_id = $1 AND namespace_id = $2
  AND limit_id = $3 AND window_start = $4
`, req.Caller.AccountID, req.Caller.NamespaceID, limit.id, limit.windowStart, req.Units, now)
		if err != nil {
			return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: consume limit %s: %w", limit.key, err)
		}
		if tag.RowsAffected() != 1 {
			return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: consume limit %s: counter disappeared", limit.key)
		}
		if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, limit_id, window_start,
    dedupe_key, event_kind, metric, quantity, usage_detail, occurred_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'consume', $8, $9,
          jsonb_build_object('entry_role', 'limit_window', 'limit_key', $10::text), $11)
`, ids.New("use"), req.Caller.AccountID, req.Caller.NamespaceID,
			decision.InvocationID, limit.id, limit.windowStart,
			limitUsageDedupe("consume", decision.InvocationID, limit.id, limit.windowStart),
			req.Metric, req.Units, limit.key, now); err != nil {
			return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: append limit usage %s: %w", limit.key, err)
		}
	}

	usageDetail, err := json.Marshal(map[string]any{"entry_role": "logical"})
	if err != nil {
		return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: encode usage detail: %w", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.usage_entries (
    id, account_id, namespace_id, invocation_id, dedupe_key,
    event_kind, metric, quantity, usage_detail, occurred_at
) VALUES ($1, $2, $3, $4, $5, 'consume', $6, $7, $8, $9)
`, ids.New("use"), req.Caller.AccountID, req.Caller.NamespaceID,
		decision.InvocationID, "consume:"+decision.InvocationID, req.Metric,
		req.Units, usageDetail, now); err != nil {
		return corev2.Decision{}, nil, fmt.Errorf("v2 postgres: append consume usage: %w", err)
	}

	if err := finishInvocation(ctx, db, req, decision, now, "settled", ""); err != nil {
		return corev2.Decision{}, nil, err
	}
	if err := appendAdmissionAudit(ctx, db, req, decision, now, "allowed"); err != nil {
		return corev2.Decision{}, nil, err
	}
	return decision, nil, nil
}

func loadPriorDecision(ctx context.Context, db DBTX, req corev2.ConsumeRequest, requestHash []byte) (corev2.Decision, bool, error) {
	var invocationID, status string
	var storedHash, rawDecision []byte
	err := db.QueryRow(ctx, `
SELECT id, request_hash, status, decision
FROM kave_v2.invocations
WHERE account_id = $1 AND namespace_id = $2
  AND operation = 'consume' AND idempotency_key = $3
FOR UPDATE
`, req.Caller.AccountID, req.Caller.NamespaceID, req.IdempotencyKey).
		Scan(&invocationID, &storedHash, &status, &rawDecision)
	if errors.Is(err, pgx.ErrNoRows) {
		return corev2.Decision{}, false, nil
	}
	if err != nil {
		return corev2.Decision{}, false, fmt.Errorf("v2 postgres: load idempotent invocation: %w", err)
	}
	if !bytes.Equal(storedHash, requestHash) {
		return corev2.Decision{}, false, &corev2.IdempotencyConflictError{Key: req.IdempotencyKey}
	}
	var decision corev2.Decision
	if err := json.Unmarshal(rawDecision, &decision); err != nil {
		return corev2.Decision{}, false, fmt.Errorf("v2 postgres: decode prior decision: %w", err)
	}
	if decision.InvocationID == "" {
		decision.InvocationID = invocationID
	}
	if decision.Status == "" {
		return corev2.Decision{}, false, fmt.Errorf("v2 postgres: prior invocation %s has no terminal decision (status=%s)", invocationID, status)
	}
	return decision, true, nil
}

func resolveAgent(ctx context.Context, db DBTX, req corev2.ConsumeRequest) (string, error) {
	var id string
	err := db.QueryRow(ctx, `
SELECT id
FROM kave_v2.agents
WHERE account_id = $1 AND namespace_id = $2
  AND name = $3 AND status = 'active'
`, req.Caller.AccountID, req.Caller.NamespaceID, req.Agent).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: agent %q is not active in this namespace", corev2.ErrInvalidArgument, req.Agent)
	}
	if err != nil {
		return "", fmt.Errorf("v2 postgres: resolve agent: %w", err)
	}
	return id, nil
}

type applicableLimit struct {
	id            string
	key           string
	hardCap       int64
	softCap       *int64
	windowKind    string
	windowSeconds *int64
	windowAnchor  *time.Time
	effectiveFrom *time.Time
	effectiveTo   *time.Time
	windowStart   time.Time
	windowEnd     time.Time
	used          int64
	reserved      int64
}

func loadApplicableLimits(ctx context.Context, db DBTX, req corev2.ConsumeRequest, agentID string, now time.Time) ([]applicableLimit, error) {
	rows, err := db.Query(ctx, `
SELECT id, external_key, hard_cap, soft_cap, window_kind,
       window_seconds, window_anchor, effective_from, effective_to
FROM kave_v2.limits
WHERE account_id = $1 AND namespace_id = $2
  AND enabled
  AND superseded_at IS NULL
  AND metric = $3
  AND (tenant_ref IS NULL OR tenant_ref = NULLIF($4, ''))
  AND (actor_ref IS NULL OR actor_ref = NULLIF($5, ''))
  AND (billing_ref IS NULL OR billing_ref = NULLIF($6, ''))
  AND (agent_id IS NULL OR agent_id = $7)
  AND (model IS NULL OR model = NULLIF($8, ''))
  AND (feature IS NULL OR feature = NULLIF($9, ''))
  AND (effective_from IS NULL OR effective_from <= $10)
  AND (effective_to IS NULL OR effective_to > $10)
ORDER BY id
FOR UPDATE
`, req.Caller.AccountID, req.Caller.NamespaceID, req.Metric,
		req.Scope.Tenant, req.Scope.Actor, req.Scope.BillTo, agentID,
		req.Model, req.Scope.Feature, now)
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: query applicable limits: %w", err)
	}
	defer rows.Close()

	limits := make([]applicableLimit, 0)
	for rows.Next() {
		var limit applicableLimit
		if err := rows.Scan(&limit.id, &limit.key, &limit.hardCap, &limit.softCap,
			&limit.windowKind, &limit.windowSeconds, &limit.windowAnchor,
			&limit.effectiveFrom, &limit.effectiveTo); err != nil {
			return nil, fmt.Errorf("v2 postgres: scan applicable limit: %w", err)
		}
		limits = append(limits, limit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("v2 postgres: iterate applicable limits: %w", err)
	}
	return limits, nil
}

func admissionWindow(now time.Time, limit applicableLimit) (time.Time, time.Time, error) {
	now = now.UTC()
	switch limit.windowKind {
	case "calendar_day":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1), nil
	case "calendar_month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), nil
	case "fixed":
		if limit.windowSeconds == nil || limit.windowAnchor == nil || *limit.windowSeconds <= 0 {
			return time.Time{}, time.Time{}, fmt.Errorf("v2 postgres: limit %s has invalid fixed window", limit.key)
		}
		anchor := limit.windowAnchor.UTC()
		elapsed := now.Unix() - anchor.Unix()
		bucket := elapsed / *limit.windowSeconds
		if elapsed < 0 && elapsed%*limit.windowSeconds != 0 {
			// Go integer division truncates toward zero; fixed windows require
			// mathematical floor so timestamps before the anchor land in the
			// preceding bucket rather than a future one.
			bucket--
		}
		start := time.Unix(anchor.Unix()+bucket*(*limit.windowSeconds), 0).UTC()
		return start, time.Unix(start.Unix()+*limit.windowSeconds, 0).UTC(), nil
	case "explicit":
		if limit.effectiveFrom == nil || limit.effectiveTo == nil {
			return time.Time{}, time.Time{}, fmt.Errorf("v2 postgres: limit %s has invalid explicit window", limit.key)
		}
		return limit.effectiveFrom.UTC(), limit.effectiveTo.UTC(), nil
	case "lifetime":
		start := time.Unix(0, 0).UTC()
		if limit.effectiveFrom != nil {
			start = limit.effectiveFrom.UTC()
		}
		end := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		if limit.effectiveTo != nil {
			end = limit.effectiveTo.UTC()
		}
		return start, end, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("v2 postgres: limit %s has unsupported window %q", limit.key, limit.windowKind)
	}
}

func finishInvocation(ctx context.Context, db DBTX, req corev2.ConsumeRequest, decision corev2.Decision, now time.Time, status, rejectionCode string) error {
	rawDecision, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("v2 postgres: encode decision: %w", err)
	}
	if len(rawDecision) > 16*1024 {
		return fmt.Errorf("v2 postgres: decision exceeds 16 KiB")
	}
	var admittedAt any
	if status != "rejected" {
		admittedAt = now
	}
	var rejection any
	if rejectionCode != "" {
		rejection = rejectionCode
	}
	tag, err := db.Exec(ctx, `
UPDATE kave_v2.invocations
SET status = $5, rejection_code = $6, decision = $7,
    admitted_at = $8, finished_at = $9
WHERE account_id = $1 AND namespace_id = $2
  AND service_key_id = $3 AND id = $4
`, req.Caller.AccountID, req.Caller.NamespaceID, req.Caller.ServiceKeyID,
		decision.InvocationID, status, rejection, rawDecision, admittedAt, now)
	if err != nil {
		return fmt.Errorf("v2 postgres: finish invocation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("v2 postgres: finish invocation: invocation disappeared")
	}
	return nil
}

func appendAdmissionAudit(ctx context.Context, db DBTX, req corev2.ConsumeRequest, decision corev2.Decision, now time.Time, outcome string) error {
	details, err := json.Marshal(map[string]any{
		"metric":          req.Metric,
		"units":           req.Units,
		"decision_status": decision.Status,
		"violation_count": len(decision.Violations),
	})
	if err != nil {
		return fmt.Errorf("v2 postgres: encode admission audit: %w", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.audit_events (
    id, account_id, namespace_id, service_key_id,
    event, resource_type, resource_id, outcome, details, created_at
) VALUES ($1, $2, $3, $4, 'admission.consume', 'invocation', $5, $6, $7, $8)
`, ids.New("aud"), req.Caller.AccountID, req.Caller.NamespaceID,
		req.Caller.ServiceKeyID, decision.InvocationID, outcome, details, now); err != nil {
		return fmt.Errorf("v2 postgres: append admission audit: %w", err)
	}
	return nil
}

func decisionViolatesLimit(decision corev2.Decision, limitID string) bool {
	for _, violation := range decision.Violations {
		if violation.LimitID == limitID {
			return true
		}
	}
	return false
}

func limitUsageDedupe(kind, invocationID, limitID string, windowStart time.Time) string {
	return kind + ":" + invocationID + ":limit:" + limitID + ":" + windowStart.UTC().Format(time.RFC3339Nano)
}

func addWouldOverflow(values ...int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value > 0 && total > math.MaxInt64-value {
			return 0, true
		}
		total += value
	}
	return total, false
}

func isRetryableTransactionError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01" || pgErr.Code == "23505")
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

var _ corev2.AdmissionStore = (*AdmissionStore)(nil)
