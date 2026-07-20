package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

const maxSyncLimitsAttempts = 8

// LimitSyncStore materializes one external owner's complete desired limit set.
// It serializes with Apply and admission on the namespace row, while retaining
// immutable generations for every changed definition.
type LimitSyncStore struct {
	runner *ScopedRunner
}

func NewLimitSyncStore(pool *pgxpool.Pool) (*LimitSyncStore, error) {
	runner, err := NewScopedRunner(pool)
	if err != nil {
		return nil, err
	}
	return &LimitSyncStore{runner: runner}, nil
}

func (s *LimitSyncStore) SyncLimits(ctx context.Context, req corev2.SyncLimitsRequest) (corev2.SyncLimitsResult, error) {
	if s == nil || s.runner == nil {
		return corev2.SyncLimitsResult{}, ErrNilPool
	}
	if err := req.ValidateRequest(); err != nil {
		return corev2.SyncLimitsResult{}, err
	}
	requestHash, err := req.Hash()
	if err != nil {
		return corev2.SyncLimitsResult{}, err
	}
	canonical := req
	canonical.Limits = req.CanonicalLimits()

	for attempt := 1; attempt <= maxSyncLimitsAttempts; attempt++ {
		var result corev2.SyncLimitsResult
		err = s.runner.WithScope(ctx, Scope{
			AccountID: string(req.Caller.AccountID), NamespaceID: string(req.NamespaceID),
		}, func(txCtx context.Context, db DBTX) error {
			var txErr error
			result, txErr = syncLimitsTx(txCtx, db, canonical, requestHash)
			return txErr
		})
		if err == nil {
			return result, nil
		}
		if !isRetryableTransactionError(err) || attempt == maxSyncLimitsAttempts {
			return corev2.SyncLimitsResult{}, err
		}
	}
	return corev2.SyncLimitsResult{}, err
}

func syncLimitsTx(ctx context.Context, db DBTX, req corev2.SyncLimitsRequest, requestHash string) (corev2.SyncLimitsResult, error) {
	if err := lockActiveSyncNamespace(ctx, db, req); err != nil {
		return corev2.SyncLimitsResult{}, err
	}

	auditID := syncLimitsAuditID(req.Caller.AccountID, req.NamespaceID, req.IdempotencyKey)
	prior, found, err := loadSyncLimitsAuditByID(ctx, db, req, auditID)
	if err != nil {
		return corev2.SyncLimitsResult{}, err
	}
	if found {
		if prior.RequestHash != requestHash {
			return corev2.SyncLimitsResult{}, &corev2.IdempotencyConflictError{Key: req.IdempotencyKey}
		}
		result := prior.Result
		result.Replayed = true
		return result, nil
	}

	latest, hasLatest, err := loadLatestSyncLimitsAudit(ctx, db, req)
	if err != nil {
		return corev2.SyncLimitsResult{}, err
	}
	if hasLatest && req.Revision <= latest.SourceRevision {
		if req.Revision != latest.SourceRevision || requestHash != latest.RequestHash {
			return corev2.SyncLimitsResult{}, &corev2.SourceRevisionConflictError{
				Owner: req.Owner, Requested: req.Revision, Current: latest.SourceRevision,
			}
		}
		result := latest.Result
		result.Replayed = true
		if err := appendSyncLimitsAudit(ctx, db, req, auditID, requestHash, result); err != nil {
			return corev2.SyncLimitsResult{}, err
		}
		return result, nil
	}

	state, err := loadSyncLimitState(ctx, db, req)
	if err != nil {
		return corev2.SyncLimitsResult{}, err
	}
	// Audit is authoritative because it also records empty desired sets. The
	// source_version fallback fails closed if rows predate that audit contract.
	if state.maxSourceRevision > 0 && (!hasLatest || state.maxSourceRevision > latest.SourceRevision) && req.Revision <= state.maxSourceRevision {
		return corev2.SyncLimitsResult{}, &corev2.SourceRevisionConflictError{
			Owner: req.Owner, Requested: req.Revision, Current: state.maxSourceRevision,
		}
	}

	result, err := reconcileSyncedLimits(ctx, db, req, state)
	if err != nil {
		return corev2.SyncLimitsResult{}, err
	}
	if err := appendSyncLimitsAudit(ctx, db, req, auditID, requestHash, result); err != nil {
		return corev2.SyncLimitsResult{}, err
	}
	return result, nil
}

func lockActiveSyncNamespace(ctx context.Context, db DBTX, req corev2.SyncLimitsRequest) error {
	var status string
	err := db.QueryRow(ctx, `
SELECT status
FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
FOR UPDATE
`, req.Caller.AccountID, req.NamespaceID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: namespace %q does not exist", corev2.ErrInvalidArgument, req.NamespaceID)
	}
	if err != nil {
		return fmt.Errorf("v2 postgres: lock namespace for limit synchronization: %w", err)
	}
	if status != "active" {
		return fmt.Errorf("%w: namespace %q is not active", corev2.ErrInvalidArgument, req.NamespaceID)
	}
	return nil
}

type syncLimitKeyState struct {
	maxGeneration int64
	current       *limitRow
}

type syncLimitState struct {
	agents            map[string]string
	keys              map[string]*syncLimitKeyState
	maxSourceRevision int64
}

func loadSyncLimitState(ctx context.Context, db DBTX, req corev2.SyncLimitsRequest) (*syncLimitState, error) {
	state := &syncLimitState{agents: map[string]string{}, keys: map[string]*syncLimitKeyState{}}
	agentRows, err := db.Query(ctx, `
SELECT name, id
FROM kave_v2.agents
WHERE account_id = $1 AND namespace_id = $2
  AND status <> 'archived'
`, req.Caller.AccountID, req.NamespaceID)
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: list agents for limit synchronization: %w", err)
	}
	for agentRows.Next() {
		var name, id string
		if err := agentRows.Scan(&name, &id); err != nil {
			agentRows.Close()
			return nil, fmt.Errorf("v2 postgres: scan agent for limit synchronization: %w", err)
		}
		state.agents[name] = id
	}
	if err := agentRows.Err(); err != nil {
		agentRows.Close()
		return nil, fmt.Errorf("v2 postgres: iterate agents for limit synchronization: %w", err)
	}
	agentRows.Close()

	rows, err := db.Query(ctx, `
SELECT l.id, l.external_key, l.generation, l.source, l.source_version, l.metric,
       COALESCE(l.tenant_ref, ''), COALESCE(l.actor_ref, ''), COALESCE(l.billing_ref, ''),
       COALESCE(l.agent_id, ''), COALESCE(a.name, ''), COALESCE(l.model, ''), COALESCE(l.feature, ''),
       l.window_kind, l.hard_cap, l.soft_cap, l.enabled,
       l.superseded_at IS NOT NULL, l.revision
FROM kave_v2.limits l
LEFT JOIN kave_v2.agents a
  ON a.account_id = l.account_id AND a.namespace_id = l.namespace_id AND a.id = l.agent_id
WHERE l.account_id = $1 AND l.namespace_id = $2
ORDER BY l.external_key, l.generation
`, req.Caller.AccountID, req.NamespaceID)
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: list limits for synchronization: %w", err)
	}
	for rows.Next() {
		var row limitRow
		var sourceVersion string
		var soft sql.NullInt64
		if err := rows.Scan(&row.id, &row.key, &row.generation, &row.source, &sourceVersion, &row.metric,
			&row.tenant, &row.actor, &row.billTo, &row.agentID, &row.agentName,
			&row.model, &row.feature, &row.window, &row.hardCap, &soft, &row.enabled,
			&row.superseded, &row.revision); err != nil {
			rows.Close()
			return nil, fmt.Errorf("v2 postgres: scan limit for synchronization: %w", err)
		}
		if soft.Valid {
			value := soft.Int64
			row.softCap = &value
		}
		keyState := state.keys[row.key]
		if keyState == nil {
			keyState = &syncLimitKeyState{}
			state.keys[row.key] = keyState
		}
		if row.generation > keyState.maxGeneration {
			keyState.maxGeneration = row.generation
		}
		if !row.superseded {
			current := row
			keyState.current = &current
		}
		if row.source == string(req.Owner) && sourceVersion != "" {
			revision, err := strconv.ParseInt(sourceVersion, 10, 64)
			if err != nil || revision <= 0 {
				rows.Close()
				return nil, fmt.Errorf("v2 postgres: corrupt source revision for owner %q", req.Owner)
			}
			if revision > state.maxSourceRevision {
				state.maxSourceRevision = revision
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("v2 postgres: iterate limits for synchronization: %w", err)
	}
	rows.Close()
	return state, nil
}

func reconcileSyncedLimits(ctx context.Context, db DBTX, req corev2.SyncLimitsRequest, state *syncLimitState) (corev2.SyncLimitsResult, error) {
	result := corev2.SyncLimitsResult{Revision: req.Revision}
	desired := make(map[string]struct{}, len(req.Limits))
	sourceVersion := strconv.FormatInt(req.Revision, 10)

	for _, spec := range req.Limits {
		key := string(spec.Key)
		desired[key] = struct{}{}
		keyState := state.keys[key]
		if keyState != nil && keyState.current != nil && keyState.current.source != string(req.Owner) {
			return corev2.SyncLimitsResult{}, &corev2.LimitOwnershipConflictError{Key: spec.Key}
		}

		agentID := ""
		if spec.Selector.Agent != "" {
			var exists bool
			agentID, exists = state.agents[string(spec.Selector.Agent)]
			if !exists {
				return corev2.SyncLimitsResult{}, fmt.Errorf("%w: limit %q references missing agent %q",
					corev2.ErrInvalidArgument, spec.Key, spec.Selector.Agent)
			}
		}
		window := limitWindowKind(spec.Window)
		if keyState == nil {
			if err := insertSyncedLimit(ctx, db, req, spec, agentID, window, sourceVersion, 1); err != nil {
				return corev2.SyncLimitsResult{}, err
			}
			result.Created++
			continue
		}
		if keyState.current == nil {
			if err := insertSyncedLimit(ctx, db, req, spec, agentID, window, sourceVersion, keyState.maxGeneration+1); err != nil {
				return corev2.SyncLimitsResult{}, err
			}
			result.Updated++
			continue
		}
		fields := changedLimitFields(*keyState.current, spec, agentID, window)
		if len(fields) == 0 {
			continue
		}
		if limitPolicyOnlyChange(fields) {
			if err := updateCurrentLimitPolicy(ctx, db, req.Caller.AccountID, req.NamespaceID,
				keyState.current.id, spec, &sourceVersion); err != nil {
				return corev2.SyncLimitsResult{}, fmt.Errorf("v2 postgres: update synchronized limit policy %q: %w", spec.Key, err)
			}
			result.Updated++
			continue
		}
		if err := archiveSyncedLimit(ctx, db, req, keyState.current.id); err != nil {
			return corev2.SyncLimitsResult{}, fmt.Errorf("v2 postgres: archive changed synchronized limit %q: %w", spec.Key, err)
		}
		if err := insertSyncedLimit(ctx, db, req, spec, agentID, window, sourceVersion, keyState.maxGeneration+1); err != nil {
			return corev2.SyncLimitsResult{}, err
		}
		result.Updated++
	}

	omitted := make([]string, 0)
	for key, keyState := range state.keys {
		if keyState.current == nil || keyState.current.source != string(req.Owner) {
			continue
		}
		if _, keep := desired[key]; !keep {
			omitted = append(omitted, key)
		}
	}
	slices.Sort(omitted)
	for _, key := range omitted {
		if err := archiveSyncedLimit(ctx, db, req, state.keys[key].current.id); err != nil {
			return corev2.SyncLimitsResult{}, fmt.Errorf("v2 postgres: disable omitted synchronized limit %q: %w", key, err)
		}
		result.Disabled++
	}
	return result, nil
}

func archiveSyncedLimit(ctx context.Context, db DBTX, req corev2.SyncLimitsRequest, id string) error {
	tag, err := db.Exec(ctx, `
UPDATE kave_v2.limits
SET enabled = FALSE, superseded_at = transaction_timestamp()
WHERE account_id = $1 AND namespace_id = $2 AND id = $3 AND superseded_at IS NULL
`, req.Caller.AccountID, req.NamespaceID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("expected to archive one current limit, archived %d", tag.RowsAffected())
	}
	return nil
}

func insertSyncedLimit(
	ctx context.Context,
	db DBTX,
	req corev2.SyncLimitsRequest,
	spec corev2.LimitSpec,
	agentID, window, sourceVersion string,
	generation int64,
) error {
	_, err := db.Exec(ctx, `
INSERT INTO kave_v2.limits (
    id, account_id, namespace_id, external_key, generation, source, source_version, metric,
    tenant_ref, actor_ref, billing_ref, agent_id, model, feature,
    hard_cap, soft_cap, window_kind, enabled
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, ''),
    $15, $16, $17, $18
)
`, ids.New("lim"), req.Caller.AccountID, req.NamespaceID, spec.Key, generation,
		req.Owner, sourceVersion, spec.Metric,
		spec.Selector.Tenant, spec.Selector.Actor, spec.Selector.BillTo, agentID,
		spec.Selector.Model, spec.Selector.Feature, spec.HardCap, spec.SoftCap, window, spec.Enabled)
	if err != nil {
		return fmt.Errorf("v2 postgres: insert synchronized limit %q generation %d: %w", spec.Key, generation, err)
	}
	return nil
}

type syncLimitsAuditDetails struct {
	RequestHash         string                  `json:"request_hash"`
	Owner               string                  `json:"owner"`
	SourceRevision      int64                   `json:"source_revision"`
	RequestedLimitCount int                     `json:"requested_limit_count"`
	Result              corev2.SyncLimitsResult `json:"result"`
}

func (d syncLimitsAuditDetails) validate() error {
	if len(d.RequestHash) != sha256.Size*2 || corev2.Ref(d.Owner).Validate("audit.owner", true) != nil || d.SourceRevision <= 0 ||
		d.Result.Revision != d.SourceRevision || d.RequestedLimitCount < 0 ||
		d.Result.Created < 0 || d.Result.Updated < 0 || d.Result.Disabled < 0 {
		return errors.New("v2 postgres: corrupt limit synchronization audit")
	}
	return nil
}

func loadSyncLimitsAuditByID(ctx context.Context, db DBTX, req corev2.SyncLimitsRequest, id string) (syncLimitsAuditDetails, bool, error) {
	var raw []byte
	err := db.QueryRow(ctx, `
SELECT details
FROM kave_v2.audit_events
WHERE account_id = $1 AND namespace_id = $2 AND id = $3 AND event = 'limits.sync'
`, req.Caller.AccountID, req.NamespaceID, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return syncLimitsAuditDetails{}, false, nil
	}
	if err != nil {
		return syncLimitsAuditDetails{}, false, fmt.Errorf("v2 postgres: load limit synchronization idempotency: %w", err)
	}
	var details syncLimitsAuditDetails
	if err := json.Unmarshal(raw, &details); err != nil {
		return syncLimitsAuditDetails{}, false, fmt.Errorf("v2 postgres: decode limit synchronization idempotency: %w", err)
	}
	if err := details.validate(); err != nil {
		return syncLimitsAuditDetails{}, false, err
	}
	return details, true, nil
}

func loadLatestSyncLimitsAudit(ctx context.Context, db DBTX, req corev2.SyncLimitsRequest) (syncLimitsAuditDetails, bool, error) {
	var raw []byte
	err := db.QueryRow(ctx, `
SELECT details
FROM kave_v2.audit_events
WHERE account_id = $1 AND namespace_id = $2
  AND event = 'limits.sync' AND resource_type = 'limit_source' AND resource_id = $3
  AND outcome = 'succeeded'
ORDER BY (details->>'source_revision')::bigint DESC, created_at DESC, id DESC
LIMIT 1
`, req.Caller.AccountID, req.NamespaceID, req.Owner).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return syncLimitsAuditDetails{}, false, nil
	}
	if err != nil {
		return syncLimitsAuditDetails{}, false, fmt.Errorf("v2 postgres: load latest limit source revision: %w", err)
	}
	var details syncLimitsAuditDetails
	if err := json.Unmarshal(raw, &details); err != nil {
		return syncLimitsAuditDetails{}, false, fmt.Errorf("v2 postgres: decode latest limit source revision: %w", err)
	}
	if err := details.validate(); err != nil {
		return syncLimitsAuditDetails{}, false, err
	}
	if details.Owner != string(req.Owner) {
		return syncLimitsAuditDetails{}, false, errors.New("v2 postgres: corrupt limit synchronization owner")
	}
	return details, true, nil
}

func appendSyncLimitsAudit(
	ctx context.Context,
	db DBTX,
	req corev2.SyncLimitsRequest,
	id, requestHash string,
	result corev2.SyncLimitsResult,
) error {
	details, err := json.Marshal(syncLimitsAuditDetails{
		RequestHash: requestHash, Owner: string(req.Owner), SourceRevision: req.Revision,
		RequestedLimitCount: len(req.Limits), Result: result,
	})
	if err != nil {
		return fmt.Errorf("v2 postgres: encode limit synchronization audit: %w", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.audit_events (
    id, account_id, namespace_id, service_key_id,
    event, resource_type, resource_id, outcome, request_id, details
) VALUES (
    $1, $2, $3, $4,
    'limits.sync', 'limit_source', $5, 'succeeded', $6, $7
)
`, id, req.Caller.AccountID, req.NamespaceID, req.Caller.ServiceKeyID,
		req.Owner, req.IdempotencyKey, details); err != nil {
		return fmt.Errorf("v2 postgres: append limit synchronization audit: %w", err)
	}
	return nil
}

func syncLimitsAuditID(accountID, namespaceID, key corev2.Ref) string {
	digest := sha256.Sum256([]byte(string(accountID) + "\x00" + string(namespaceID) + "\x00" + string(key)))
	return "aud_sync_" + fmt.Sprintf("%x", digest[:])
}
