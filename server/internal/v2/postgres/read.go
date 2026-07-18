package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	corev2 "github.com/kave-io/kave/core/v2"
)

// ReadStore serves the bounded, namespace-scoped control and reporting reads.
// It never selects provider request/response bodies, secret storage columns,
// key hashes, or raw service-key material.
type ReadStore struct {
	runner *ScopedRunner
	now    func() time.Time
}

func NewReadStore(pool *pgxpool.Pool) (*ReadStore, error) {
	runner, err := NewScopedRunner(pool)
	if err != nil {
		return nil, err
	}
	return &ReadStore{runner: runner, now: time.Now}, nil
}

func (s *ReadStore) GetState(ctx context.Context, req corev2.GetStateRequest) (corev2.State, error) {
	if err := req.Validate(); err != nil {
		return corev2.State{}, err
	}
	var result corev2.State
	err := s.runner.withScope(ctx, readScope(req.Caller), pgx.ReadCommitted, func(txCtx context.Context, db DBTX) error {
		var status string
		if err := db.QueryRow(txCtx, `
SELECT application, environment, revision, status
FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
`, req.Caller.AccountID, req.NamespaceID).Scan(
			&result.Manifest.Namespace.Application, &result.Manifest.Namespace.Environment,
			&result.Revision, &status,
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return corev2.ErrNamespaceNotFound
			}
			return fmt.Errorf("v2 postgres: load namespace state: %w", err)
		}
		if status != "active" {
			return corev2.ErrNamespaceNotFound
		}
		result.NamespaceID = req.NamespaceID
		result.Manifest.Namespace.ID = string(req.NamespaceID)
		result.Manifest.Namespace.Account = req.Caller.AccountID

		state, err := loadApplyState(txCtx, db, req.Caller.AccountID, req.NamespaceID)
		if err != nil {
			return err
		}
		result.Manifest.Routes = activeManifestRoutes(state)
		result.Manifest.Agents = manifestAgents(state)
		result.Manifest.Limits = operatorManifestLimits(state)
		return nil
	})
	return result, err
}

func activeManifestRoutes(state *applyState) []corev2.RouteSpec {
	routes := make([]corev2.RouteSpec, 0, len(state.routes))
	for _, route := range state.routes {
		if route.status != "active" {
			continue
		}
		routes = append(routes, corev2.RouteSpec{
			Name: corev2.Ref(route.name), Provider: corev2.Ref(route.provider), BaseURL: route.baseURL,
			Secret: corev2.Ref(route.secretName), AllowedModels: slices.Clone(route.allowedModels),
			DefaultModel: route.defaultModel, PricingRevision: route.pricingRevision,
			Pricing: slices.Clone(route.pricing),
		})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Name < routes[j].Name })
	return routes
}

func manifestAgents(state *applyState) []corev2.AgentSpec {
	agents := make([]corev2.AgentSpec, 0, len(state.agents))
	for _, agent := range state.agents {
		if agent.status == "archived" {
			continue
		}
		route, ok := state.routes[agent.routeName]
		if !ok || route.status != "active" {
			continue
		}
		agents = append(agents, corev2.AgentSpec{
			Name: corev2.Ref(agent.name), Kind: corev2.AgentKind(agent.kind),
			Route: corev2.Ref(agent.routeName), Enabled: agent.status == "active",
		})
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents
}

func operatorManifestLimits(state *applyState) []corev2.LimitSpec {
	limits := make([]corev2.LimitSpec, 0, len(state.limits))
	for _, limit := range state.limits {
		if limit.source != "operator" || limit.superseded {
			continue
		}
		limits = append(limits, corev2.LimitSpec{
			Key: corev2.Ref(limit.key), Metric: corev2.Metric(limit.metric),
			Selector: corev2.LimitSelector{
				Tenant: corev2.Ref(limit.tenant), Actor: corev2.Ref(limit.actor),
				BillTo: corev2.Ref(limit.billTo), Agent: corev2.Ref(limit.agentName),
				Model: corev2.Ref(limit.model), Feature: corev2.Ref(limit.feature),
			},
			Window: manifestWindow(limit.window), HardCap: limit.hardCap,
			SoftCap: cloneInt64(limit.softCap), Enabled: limit.enabled,
		})
	}
	sort.Slice(limits, func(i, j int) bool { return limits[i].Key < limits[j].Key })
	return limits
}

func manifestWindow(window string) corev2.Window {
	switch window {
	case "calendar_day":
		return corev2.WindowDay
	case "calendar_month":
		return corev2.WindowMonth
	case "lifetime":
		return corev2.WindowAllTime
	default:
		// Apply currently persists only these three window types. Returning an
		// empty value makes an unexpected database state visible to validation.
		return ""
	}
}

func (s *ReadStore) GetLimitStatus(ctx context.Context, req corev2.GetLimitStatusRequest) ([]corev2.LimitStatus, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	result := make([]corev2.LimitStatus, 0)
	err := s.runner.withScope(ctx, readScope(req.Caller), pgx.ReadCommitted, func(txCtx context.Context, db DBTX) error {
		if err := requireActiveReadNamespace(txCtx, db, req.Caller); err != nil {
			return err
		}
		agentID, err := resolveReadAgent(txCtx, db, req.Caller, req.Agent)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		rows, err := db.Query(txCtx, `
SELECT id, external_key, hard_cap, soft_cap, window_kind,
       window_seconds, window_anchor, effective_from, effective_to
FROM kave_v2.limits
WHERE account_id = $1 AND namespace_id = $2
  AND enabled AND superseded_at IS NULL AND metric = $3
  AND (tenant_ref IS NULL OR tenant_ref = $4)
  AND (actor_ref IS NULL OR actor_ref = NULLIF($5, ''))
  AND (billing_ref IS NULL OR billing_ref = $6)
  AND (agent_id IS NULL OR agent_id = $7)
  AND (model IS NULL OR model = NULLIF($8, ''))
  AND (feature IS NULL OR feature = NULLIF($9, ''))
  AND (effective_from IS NULL OR effective_from <= $10)
  AND (effective_to IS NULL OR effective_to > $10)
ORDER BY external_key, id
`, req.Caller.AccountID, req.Caller.NamespaceID, req.Metric,
			req.Scope.Tenant, req.Scope.Actor, req.Scope.BillTo, agentID,
			req.Model, req.Scope.Feature, now)
		if err != nil {
			return fmt.Errorf("v2 postgres: query limit status: %w", err)
		}
		limits := make([]applicableLimit, 0)
		for rows.Next() {
			var limit applicableLimit
			if err := rows.Scan(&limit.id, &limit.key, &limit.hardCap, &limit.softCap,
				&limit.windowKind, &limit.windowSeconds, &limit.windowAnchor,
				&limit.effectiveFrom, &limit.effectiveTo); err != nil {
				rows.Close()
				return fmt.Errorf("v2 postgres: scan limit status: %w", err)
			}
			limits = append(limits, limit)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("v2 postgres: iterate limit status: %w", err)
		}
		rows.Close()
		if len(limits) > maxApplicableLimits {
			return fmt.Errorf("v2 postgres: %d applicable limits exceed safety maximum %d", len(limits), maxApplicableLimits)
		}
		for _, limit := range limits {
			start, end, err := admissionWindow(now, limit)
			if err != nil {
				return err
			}
			var used, reserved int64
			err = db.QueryRow(txCtx, `
SELECT used, reserved
FROM kave_v2.limit_windows
WHERE account_id = $1 AND namespace_id = $2 AND limit_id = $3 AND window_start = $4
`, req.Caller.AccountID, req.Caller.NamespaceID, limit.id, start).Scan(&used, &reserved)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("v2 postgres: load limit window status: %w", err)
			}
			result = append(result, corev2.LimitStatus{
				LimitID: corev2.Ref(limit.id), LimitKey: corev2.Ref(limit.key), Metric: req.Metric,
				Used: used, Reserved: reserved, HardCap: limit.hardCap,
				SoftCap: cloneInt64(limit.softCap), ResetAt: end,
			})
		}
		return nil
	})
	return result, err
}

func resolveReadAgent(ctx context.Context, db DBTX, caller corev2.Caller, name corev2.Ref) (string, error) {
	var id string
	if err := db.QueryRow(ctx, `
SELECT id FROM kave_v2.agents
WHERE account_id = $1 AND namespace_id = $2 AND name = $3 AND status = 'active'
`, caller.AccountID, caller.NamespaceID, name).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("%w: agent %q is not active", corev2.ErrInvalidArgument, name)
		}
		return "", fmt.Errorf("v2 postgres: resolve reporting agent: %w", err)
	}
	if caller.Allows(corev2.OperationUsageRead, "") || caller.Allows(corev2.OperationConfigApply, "") {
		return id, nil
	}
	if len(caller.AllowedAgentIDs) == 0 || !slices.Contains(caller.AllowedAgentIDs, corev2.Ref(id)) {
		return "", fmt.Errorf("%w: service key cannot inspect agent %q", corev2.ErrUnauthorized, name)
	}
	return id, nil
}

func (s *ReadStore) QueryUsage(ctx context.Context, req corev2.QueryUsageRequest) (corev2.QueryUsageResult, error) {
	if err := req.Validate(); err != nil {
		return corev2.QueryUsageResult{}, err
	}
	cursor, err := decodeReadCursor(req.Page.Token, usageQueryFingerprint(req), req.Range)
	if err != nil {
		return corev2.QueryUsageResult{}, err
	}
	result := corev2.QueryUsageResult{Entries: make([]corev2.UsageEntry, 0, req.Page.EffectiveSize())}
	err = s.runner.withScope(ctx, readScope(req.Caller), pgx.ReadCommitted, func(txCtx context.Context, db DBTX) error {
		if err := requireActiveReadNamespace(txCtx, db, req.Caller); err != nil {
			return err
		}
		rows, err := db.Query(txCtx, `
SELECT u.id, u.invocation_id,
       CASE
         WHEN i.kind = 'consume' THEN COALESCE(u.metric, '')
         WHEN $9 <> '' THEN $9
         ELSE ''
       END AS metric,
       CASE
         WHEN i.kind = 'consume' THEN u.quantity
         WHEN $9 = 'requests' THEN u.request_count
         WHEN $9 = 'input_tokens' THEN u.input_tokens
         WHEN $9 = 'output_tokens' THEN u.output_tokens
         WHEN $9 = 'cost_nano_usd' THEN u.cost_nanos
         ELSE 0
       END AS quantity,
       u.request_count, u.input_tokens, u.output_tokens,
       u.cache_read_tokens, u.cache_write_tokens, u.reasoning_tokens,
	       u.cost_nanos,
	       COALESCE((u.usage_detail->>'accounting_estimate')::BOOLEAN, FALSE),
	       COALESCE(u.provider, ''), COALESCE(u.model, ''),
       u.attempt_no, u.event_kind, u.occurred_at
FROM kave_v2.usage_entries AS u
JOIN kave_v2.invocations AS i
  ON i.account_id = u.account_id AND i.namespace_id = u.namespace_id AND i.id = u.invocation_id
LEFT JOIN kave_v2.agents AS a
  ON a.account_id = i.account_id AND a.namespace_id = i.namespace_id AND a.id = i.agent_id
WHERE u.account_id = $1 AND u.namespace_id = $2
  AND i.tenant_ref = $3 AND i.billing_ref = $4
  AND ($5 = '' OR i.actor_ref = $5)
  AND ($6 = '' OR i.session_ref = $6)
  AND ($7 = '' OR i.feature = $7)
  AND ($8 = '' OR a.name = $8)
  -- Reporting exposes one canonical row per logical consumption/provider
  -- attempt. Per-limit reservation, block, and settlement rows remain internal
  -- evidence and must never multiply application billing totals.
  AND (
       (i.kind = 'consume' AND u.event_kind = 'consume'
        AND u.limit_id IS NULL AND u.usage_detail->>'entry_role' = 'logical')
       OR
       (i.kind = 'provider' AND u.event_kind = 'settlement'
        AND u.limit_id IS NULL AND u.dedupe_key LIKE 'usage:%')
  )
  AND (
       $9 = ''
       OR (i.kind = 'consume' AND u.metric = $9)
       OR (i.kind = 'provider' AND $9 = ANY(ARRAY['requests', 'input_tokens', 'output_tokens', 'cost_nano_usd']::TEXT[]))
  )
  AND u.occurred_at >= $10 AND u.occurred_at < $11
  AND ($12 OR (u.occurred_at, u.id) < ($13, $14))
ORDER BY u.occurred_at DESC, u.id DESC
LIMIT $15
`, req.Caller.AccountID, req.Caller.NamespaceID, req.Scope.Tenant, req.Scope.BillTo,
			req.Scope.Actor, req.Scope.Session, req.Scope.Feature, req.Agent, req.Metric,
			req.Range.From.UTC(), req.Range.To.UTC(), !cursor.Valid, cursor.Time, cursor.ID,
			req.Page.EffectiveSize()+1)
		if err != nil {
			return fmt.Errorf("v2 postgres: query usage: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var entry corev2.UsageEntry
			var attempt int
			if err := rows.Scan(&entry.ID, &entry.InvocationID, &entry.Metric, &entry.Quantity,
				&entry.RequestCount, &entry.InputTokens, &entry.OutputTokens,
				&entry.CacheReadTokens, &entry.CacheWriteTokens, &entry.ReasoningTokens,
				&entry.CostNanoUSD, &entry.Estimated, &entry.Provider, &entry.Model, &attempt,
				&entry.EventKind, &entry.CreatedAt); err != nil {
				return fmt.Errorf("v2 postgres: scan usage: %w", err)
			}
			entry.Attempt = int32(attempt)
			result.Entries = append(result.Entries, entry)
		}
		return rows.Err()
	})
	if err != nil {
		return corev2.QueryUsageResult{}, err
	}
	if len(result.Entries) > req.Page.EffectiveSize() {
		last := result.Entries[req.Page.EffectiveSize()-1]
		result.Entries = result.Entries[:req.Page.EffectiveSize()]
		result.NextPageToken, err = encodeReadCursor(last.CreatedAt, string(last.ID), usageQueryFingerprint(req))
	}
	return result, err
}

func (s *ReadStore) QueryInvocations(ctx context.Context, req corev2.QueryInvocationsRequest) (corev2.QueryInvocationsResult, error) {
	if err := req.Validate(); err != nil {
		return corev2.QueryInvocationsResult{}, err
	}
	cursor, err := decodeReadCursor(req.Page.Token, invocationQueryFingerprint(req), req.Range)
	if err != nil {
		return corev2.QueryInvocationsResult{}, err
	}
	result := corev2.QueryInvocationsResult{Invocations: make([]corev2.Invocation, 0, req.Page.EffectiveSize())}
	err = s.runner.withScope(ctx, readScope(req.Caller), pgx.ReadCommitted, func(txCtx context.Context, db DBTX) error {
		if err := requireActiveReadNamespace(txCtx, db, req.Caller); err != nil {
			return err
		}
		rows, err := db.Query(txCtx, `
SELECT i.id, COALESCE(a.name, ''), COALESCE(i.model, ''),
       COALESCE(i.tenant_ref, ''), COALESCE(i.actor_ref, ''), COALESCE(i.billing_ref, ''),
       COALESCE(i.session_ref, ''), COALESCE(i.feature, ''), i.status,
       i.idempotency_key, i.created_at, i.finished_at
FROM kave_v2.invocations AS i
LEFT JOIN kave_v2.agents AS a
  ON a.account_id = i.account_id AND a.namespace_id = i.namespace_id AND a.id = i.agent_id
WHERE i.account_id = $1 AND i.namespace_id = $2
  AND i.tenant_ref = $3 AND i.billing_ref = $4
  AND ($5 = '' OR i.actor_ref = $5)
  AND ($6 = '' OR i.session_ref = $6)
  AND ($7 = '' OR i.feature = $7)
  AND ($8 = '' OR a.name = $8)
  AND ($9 = '' OR ($9 = 'rejected' AND i.status = 'rejected')
       OR ($9 = 'admitted' AND i.status IN ('admitted', 'settled', 'failed', 'cancelled')))
  AND i.created_at >= $10 AND i.created_at < $11
  AND ($12 OR (i.created_at, i.id) < ($13, $14))
ORDER BY i.created_at DESC, i.id DESC
LIMIT $15
`, req.Caller.AccountID, req.Caller.NamespaceID, req.Scope.Tenant, req.Scope.BillTo,
			req.Scope.Actor, req.Scope.Session, req.Scope.Feature, req.Agent, req.Status,
			req.Range.From.UTC(), req.Range.To.UTC(), !cursor.Valid, cursor.Time, cursor.ID,
			req.Page.EffectiveSize()+1)
		if err != nil {
			return fmt.Errorf("v2 postgres: query invocations: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var invocation corev2.Invocation
			var status string
			var finished sql.NullTime
			if err := rows.Scan(&invocation.ID, &invocation.Agent, &invocation.Model,
				&invocation.Scope.Tenant, &invocation.Scope.Actor, &invocation.Scope.BillTo,
				&invocation.Scope.Session, &invocation.Scope.Feature, &status,
				&invocation.IdempotencyKey, &invocation.CreatedAt, &finished); err != nil {
				return fmt.Errorf("v2 postgres: scan invocation: %w", err)
			}
			invocation.Status = corev2.Ref(status)
			switch status {
			case "rejected":
				invocation.Decision = corev2.DecisionRejected
			case "admitted", "settled", "failed", "cancelled":
				invocation.Decision = corev2.DecisionAdmitted
			}
			if finished.Valid {
				settled := finished.Time.UTC()
				invocation.SettledAt = &settled
			}
			result.Invocations = append(result.Invocations, invocation)
		}
		return rows.Err()
	})
	if err != nil {
		return corev2.QueryInvocationsResult{}, err
	}
	if len(result.Invocations) > req.Page.EffectiveSize() {
		last := result.Invocations[req.Page.EffectiveSize()-1]
		result.Invocations = result.Invocations[:req.Page.EffectiveSize()]
		result.NextPageToken, err = encodeReadCursor(last.CreatedAt, string(last.ID), invocationQueryFingerprint(req))
	}
	return result, err
}

func (s *ReadStore) QueryAuditEvents(ctx context.Context, req corev2.QueryAuditEventsRequest) (corev2.QueryAuditEventsResult, error) {
	if err := req.Validate(); err != nil {
		return corev2.QueryAuditEventsResult{}, err
	}
	cursor, err := decodeReadCursor(req.Page.Token, auditQueryFingerprint(req), req.Range)
	if err != nil {
		return corev2.QueryAuditEventsResult{}, err
	}
	result := corev2.QueryAuditEventsResult{Events: make([]corev2.AuditEvent, 0, req.Page.EffectiveSize())}
	err = s.runner.withScope(ctx, readScope(req.Caller), pgx.ReadCommitted, func(txCtx context.Context, db DBTX) error {
		if err := requireActiveReadNamespace(txCtx, db, req.Caller); err != nil {
			return err
		}
		rows, err := db.Query(txCtx, `
SELECT id, event, COALESCE(service_key_id, ''), resource_type, resource_id,
       outcome, details, created_at
FROM kave_v2.audit_events
WHERE account_id = $1 AND namespace_id = $2
  AND ($3 = '' OR event = $3)
  AND created_at >= $4 AND created_at < $5
  AND ($6 OR (created_at, id) < ($7, $8))
ORDER BY created_at DESC, id DESC
LIMIT $9
`, req.Caller.AccountID, req.Caller.NamespaceID, req.EventKind,
			req.Range.From.UTC(), req.Range.To.UTC(), !cursor.Valid, cursor.Time, cursor.ID,
			req.Page.EffectiveSize()+1)
		if err != nil {
			return fmt.Errorf("v2 postgres: query audit events: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var event corev2.AuditEvent
			var actorID string
			var details []byte
			if err := rows.Scan(&event.ID, &event.EventKind, &actorID, &event.ResourceKind,
				&event.ResourceID, &event.Outcome, &details, &event.CreatedAt); err != nil {
				return fmt.Errorf("v2 postgres: scan audit event: %w", err)
			}
			if actorID == "" {
				event.ActorKind = "bootstrap"
			} else {
				event.ActorKind = "service_key"
				event.ActorID = corev2.Ref(actorID)
			}
			event.Metadata = safeAuditMetadata(details)
			result.Events = append(result.Events, event)
		}
		return rows.Err()
	})
	if err != nil {
		return corev2.QueryAuditEventsResult{}, err
	}
	if len(result.Events) > req.Page.EffectiveSize() {
		last := result.Events[req.Page.EffectiveSize()-1]
		result.Events = result.Events[:req.Page.EffectiveSize()]
		result.NextPageToken, err = encodeReadCursor(last.CreatedAt, string(last.ID), auditQueryFingerprint(req))
	}
	return result, err
}

func readScope(caller corev2.Caller) Scope {
	return Scope{AccountID: string(caller.AccountID), NamespaceID: string(caller.NamespaceID)}
}

func requireActiveReadNamespace(ctx context.Context, db DBTX, caller corev2.Caller) error {
	var active bool
	if err := db.QueryRow(ctx, `
SELECT status = 'active'
FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
`, caller.AccountID, caller.NamespaceID).Scan(&active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return corev2.ErrNamespaceNotFound
		}
		return fmt.Errorf("v2 postgres: check reporting namespace: %w", err)
	}
	if !active {
		return corev2.ErrUnauthorized
	}
	return nil
}

type readCursor struct {
	Version int       `json:"v"`
	Micros  int64     `json:"t"`
	ID      string    `json:"i"`
	Query   string    `json:"q"`
	Valid   bool      `json:"-"`
	Time    time.Time `json:"-"`
}

func encodeReadCursor(createdAt time.Time, id, query string) (string, error) {
	cursor := readCursor{Version: 1, Micros: createdAt.UTC().UnixMicro(), ID: id, Query: query}
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("v2 postgres: encode page cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeReadCursor(token, query string, window corev2.TimeRange) (readCursor, error) {
	if token == "" {
		return readCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) > corev2.MaxReadPageToken {
		return readCursor{}, fmt.Errorf("%w: page_token is invalid", corev2.ErrInvalidArgument)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cursor readCursor
	if err := decoder.Decode(&cursor); err != nil {
		return readCursor{}, fmt.Errorf("%w: page_token is invalid", corev2.ErrInvalidArgument)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return readCursor{}, fmt.Errorf("%w: page_token is invalid", corev2.ErrInvalidArgument)
	}
	cursor.Time = time.UnixMicro(cursor.Micros).UTC()
	if cursor.Version != 1 || cursor.Query != query || cursor.ID == "" || len(cursor.ID) > 255 ||
		cursor.Time.Before(window.From.UTC()) || !cursor.Time.Before(window.To.UTC()) {
		return readCursor{}, fmt.Errorf("%w: page_token does not belong to this query", corev2.ErrInvalidArgument)
	}
	cursor.Valid = true
	return cursor, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("trailing JSON")
	}
	return err
}

func queryFingerprint(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:16])
}

func usageQueryFingerprint(req corev2.QueryUsageRequest) string {
	return queryFingerprint("usage", string(req.Caller.NamespaceID), string(req.Scope.Tenant), string(req.Scope.Actor),
		string(req.Scope.BillTo), string(req.Scope.Session), string(req.Scope.Feature), string(req.Agent), string(req.Metric),
		strconv.FormatInt(req.Range.From.UTC().UnixMilli(), 10), strconv.FormatInt(req.Range.To.UTC().UnixMilli(), 10))
}

func invocationQueryFingerprint(req corev2.QueryInvocationsRequest) string {
	return queryFingerprint("invocations", string(req.Caller.NamespaceID), string(req.Scope.Tenant), string(req.Scope.Actor),
		string(req.Scope.BillTo), string(req.Scope.Session), string(req.Scope.Feature), string(req.Agent), string(req.Status),
		strconv.FormatInt(req.Range.From.UTC().UnixMilli(), 10), strconv.FormatInt(req.Range.To.UTC().UnixMilli(), 10))
}

func auditQueryFingerprint(req corev2.QueryAuditEventsRequest) string {
	return queryFingerprint("audit", string(req.Caller.NamespaceID), string(req.EventKind),
		strconv.FormatInt(req.Range.From.UTC().UnixMilli(), 10), strconv.FormatInt(req.Range.To.UTC().UnixMilli(), 10))
}

func safeAuditMetadata(raw []byte) map[string]string {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var values map[string]any
	if err := decoder.Decode(&values); err != nil || len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	metadata := make(map[string]string)
	for _, key := range keys {
		if len(metadata) == 32 || !safeAuditKey(key) {
			continue
		}
		var value string
		switch typed := values[key].(type) {
		case string:
			value = typed
		case bool:
			value = strconv.FormatBool(typed)
		case json.Number:
			value = typed.String()
		default:
			continue
		}
		if len(value) <= 512 && !strings.ContainsAny(value, "\x00\r\n") {
			metadata[key] = value
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func safeAuditKey(key string) bool {
	if key == "" || len(key) > 64 || strings.ContainsAny(key, "\x00\r\n") {
		return false
	}
	lower := strings.ToLower(key)
	for _, forbidden := range []string{"secret", "token", "credential", "plaintext", "ciphertext", "raw_key", "password"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	return true
}

var _ corev2.ReadStore = (*ReadStore)(nil)
