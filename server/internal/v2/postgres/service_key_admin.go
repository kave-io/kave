package postgres

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

const (
	maxServiceKeyAttempts = 8
	maxServiceKeyAgents   = 64
)

var (
	ErrServiceKeyConflict         = errors.New("v2 postgres: service key already exists with different configuration")
	ErrServiceKeyNotFound         = errors.New("v2 postgres: service key not found")
	ErrServiceKeyMaterialConflict = errors.New("v2 postgres: service-key lookup prefix is already assigned")
)

// IssueServiceKeyRequest declares one namespace-bound machine identity.
// IdempotencyKey is scoped to the namespace and survives administrator-key
// rotation; Name remains the unique stable identity of the workload key.
type IssueServiceKeyRequest struct {
	Scope              Scope
	ActingServiceKeyID corev2.Ref
	IdempotencyKey     corev2.Ref
	Name               string
	LookupPrefix       string
	SecretHash         []byte
	Operations         []corev2.Operation
	AllowedAgentNames  []corev2.Ref
	CanAssertScope     bool
	ExpiresAt          *time.Time
}

// IssuedServiceKey contains only non-secret metadata. Credential recipients
// generate and retain raw key material before calling Issue.
type IssuedServiceKey struct {
	ID              string
	Name            string
	Prefix          string
	Operations      []corev2.Operation
	AllowedAgentIDs []string
	CanAssertScope  bool
	ExpiresAt       *time.Time
	CreatedAt       time.Time
	Status          string
	Created         bool
}

type RevokeServiceKeyRequest struct {
	Scope              Scope
	ActingServiceKeyID corev2.Ref
	ServiceKeyID       corev2.Ref
	Reason             string
}

type RevokeServiceKeyResult struct {
	ServiceKeyID string
	Revoked      bool
}

// ServiceKeyAdmin owns issuance and revocation transaction boundaries. It
// persists a SHA-256 verifier and the non-secret random lookup prefix, but
// never the complete credential.
type ServiceKeyAdmin struct {
	runner *ScopedRunner
	now    func() time.Time
	newID  func(string) string
}

func NewServiceKeyAdmin(pool *pgxpool.Pool) (*ServiceKeyAdmin, error) {
	runner, err := NewScopedRunner(pool)
	if err != nil {
		return nil, err
	}
	return &ServiceKeyAdmin{
		runner: runner,
		now:    time.Now,
		newID:  ids.New,
	}, nil
}

// Issue persists a client-generated lookup prefix and SHA-256 verifier. Raw
// credential material never enters this API, so a committed response can be
// retried without a one-time-delivery failure mode.
func (a *ServiceKeyAdmin) Issue(ctx context.Context, req IssueServiceKeyRequest) (IssuedServiceKey, error) {
	normalized, err := a.validateIssue(req)
	if err != nil {
		return IssuedServiceKey{}, err
	}

	for attempt := 1; attempt <= maxServiceKeyAttempts; attempt++ {
		var result IssuedServiceKey
		err = a.runner.WithScope(ctx, normalized.Scope, func(txCtx context.Context, db DBTX) error {
			var txErr error
			result, txErr = a.issueTx(txCtx, db, normalized)
			return txErr
		})
		if err == nil {
			return result, nil
		}
		if !isRetryableTransactionError(err) || attempt == maxServiceKeyAttempts {
			return IssuedServiceKey{}, err
		}
	}
	return IssuedServiceKey{}, err
}

func (a *ServiceKeyAdmin) issueTx(ctx context.Context, db DBTX, req normalizedIssueRequest) (IssuedServiceKey, error) {
	if err := lockServiceKeyNamespace(ctx, db, req.Scope, true); err != nil {
		return IssuedServiceKey{}, err
	}
	prior, found, err := loadPriorServiceKeyIssue(ctx, db, req)
	if err != nil {
		return IssuedServiceKey{}, err
	}
	if found {
		return prior, nil
	}

	existing, found, err := loadServiceKeyByName(ctx, db, req.Scope, req.Name)
	if err != nil {
		return IssuedServiceKey{}, err
	}
	if found {
		// A replay remains idempotent if its original key has expired or an
		// allowed agent was subsequently disabled. Both conditions prevent new
		// authority, but neither may reveal fresh raw credential material.
		agentIDs, err := resolveServiceKeyAgents(ctx, db, req.Scope, req.allowedAgentNames, false)
		if err != nil {
			return IssuedServiceKey{}, err
		}
		if !existing.matches(req, agentIDs) {
			return IssuedServiceKey{}, fmt.Errorf("%w: %q", ErrServiceKeyConflict, req.Name)
		}
		result := existing.result(false)
		if err := appendServiceKeyIssueAudit(ctx, db, req, result, a.now().UTC()); err != nil {
			return IssuedServiceKey{}, err
		}
		return result, nil
	}
	if req.ExpiresAt != nil && !req.ExpiresAt.After(a.now().UTC()) {
		return IssuedServiceKey{}, fmt.Errorf("%w: service key expiration must be in the future", corev2.ErrInvalidArgument)
	}
	agentIDs, err := resolveServiceKeyAgents(ctx, db, req.Scope, req.allowedAgentNames, true)
	if err != nil {
		return IssuedServiceKey{}, err
	}

	keyID := a.newID("key")
	if keyID == "" {
		return IssuedServiceKey{}, errors.New("v2 postgres: service key id generator returned an empty id")
	}

	now := a.now().UTC()
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.service_keys (
    id, account_id, namespace_id, name, lookup_prefix, secret_hash,
    capabilities, allowed_agent_ids, can_assert_scope, expires_at, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`, keyID, req.Scope.AccountID, req.Scope.NamespaceID, req.Name,
		req.LookupPrefix, req.SecretHash[:], req.operations, agentIDs,
		req.CanAssertScope, req.ExpiresAt, now); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "service_keys_lookup_prefix_key" {
			// In SERIALIZABLE transactions, a concurrent identical issue can
			// reach the unique index using its original snapshot. Preserve the
			// Postgres cause so Issue retries and observes the idempotency row;
			// a genuine global prefix collision remains a material conflict after
			// the bounded retry loop.
			return IssuedServiceKey{}, fmt.Errorf("%w: %w", ErrServiceKeyMaterialConflict, err)
		}
		return IssuedServiceKey{}, fmt.Errorf("v2 postgres: insert service key: %w", err)
	}

	result := IssuedServiceKey{
		ID:              keyID,
		Name:            req.Name,
		Prefix:          RawServiceKeyPrefix + req.LookupPrefix,
		Operations:      stringsToOperations(req.operations),
		AllowedAgentIDs: slices.Clone(agentIDs),
		CanAssertScope:  req.CanAssertScope,
		ExpiresAt:       cloneTime(req.ExpiresAt),
		CreatedAt:       now,
		Status:          "active",
		Created:         true,
	}
	if err := appendServiceKeyIssueAudit(ctx, db, req, result, now); err != nil {
		return IssuedServiceKey{}, err
	}
	return result, nil
}

// Revoke transitions an active key to revoked. Repeating the operation for an
// already-revoked key succeeds with Revoked=false and appends no duplicate
// audit event.
func (a *ServiceKeyAdmin) Revoke(ctx context.Context, req RevokeServiceKeyRequest) (RevokeServiceKeyResult, error) {
	if a == nil || a.runner == nil || a.now == nil || a.newID == nil {
		return RevokeServiceKeyResult{}, ErrNilPool
	}
	if err := req.Scope.Validate(); err != nil {
		return RevokeServiceKeyResult{}, err
	}
	if err := req.ServiceKeyID.Validate("service_key_id", true); err != nil {
		return RevokeServiceKeyResult{}, err
	}
	if err := req.ActingServiceKeyID.Validate("acting_service_key_id", false); err != nil {
		return RevokeServiceKeyResult{}, err
	}
	if len(req.Reason) > 256 || strings.ContainsAny(req.Reason, "\r\n") {
		return RevokeServiceKeyResult{}, fmt.Errorf("%w: revoke reason must be at most 256 bytes on one line", corev2.ErrInvalidArgument)
	}

	for attempt := 1; attempt <= maxServiceKeyAttempts; attempt++ {
		var result RevokeServiceKeyResult
		err := a.runner.WithScope(ctx, req.Scope, func(txCtx context.Context, db DBTX) error {
			var txErr error
			result, txErr = a.revokeTx(txCtx, db, req)
			return txErr
		})
		if err == nil {
			return result, nil
		}
		if !isRetryableTransactionError(err) || attempt == maxServiceKeyAttempts {
			return RevokeServiceKeyResult{}, err
		}
	}
	return RevokeServiceKeyResult{}, errors.New("v2 postgres: revoke service key exhausted retries")
}

func (a *ServiceKeyAdmin) revokeTx(ctx context.Context, db DBTX, req RevokeServiceKeyRequest) (RevokeServiceKeyResult, error) {
	if err := lockServiceKeyNamespace(ctx, db, req.Scope, false); err != nil {
		return RevokeServiceKeyResult{}, err
	}

	var name, status string
	err := db.QueryRow(ctx, `
SELECT name, status
FROM kave_v2.service_keys
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
FOR UPDATE
`, req.Scope.AccountID, req.Scope.NamespaceID, req.ServiceKeyID).Scan(&name, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return RevokeServiceKeyResult{}, fmt.Errorf("%w: %q", ErrServiceKeyNotFound, req.ServiceKeyID)
	}
	if err != nil {
		return RevokeServiceKeyResult{}, fmt.Errorf("v2 postgres: load service key for revocation: %w", err)
	}
	if status == "revoked" {
		return RevokeServiceKeyResult{ServiceKeyID: string(req.ServiceKeyID)}, nil
	}
	if status != "active" {
		return RevokeServiceKeyResult{}, fmt.Errorf("v2 postgres: service key %q has invalid status %q", req.ServiceKeyID, status)
	}

	now := a.now().UTC()
	tag, err := db.Exec(ctx, `
UPDATE kave_v2.service_keys
SET status = 'revoked', revoked_at = $4
WHERE account_id = $1 AND namespace_id = $2 AND id = $3 AND status = 'active'
`, req.Scope.AccountID, req.Scope.NamespaceID, req.ServiceKeyID, now)
	if err != nil {
		return RevokeServiceKeyResult{}, fmt.Errorf("v2 postgres: revoke service key: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return RevokeServiceKeyResult{}, fmt.Errorf("v2 postgres: revoke service key %q: row disappeared", req.ServiceKeyID)
	}
	if err := appendServiceKeyAudit(ctx, db, req.Scope, req.ActingServiceKeyID,
		"service_key.revoked", string(req.ServiceKeyID), name,
		map[string]any{"name": name, "reason": req.Reason}, now); err != nil {
		return RevokeServiceKeyResult{}, err
	}
	return RevokeServiceKeyResult{ServiceKeyID: string(req.ServiceKeyID), Revoked: true}, nil
}

type normalizedIssueRequest struct {
	Scope              Scope
	ActingServiceKeyID corev2.Ref
	IdempotencyKey     corev2.Ref
	requestHash        string
	Name               string
	LookupPrefix       string
	SecretHash         [sha256.Size]byte
	operations         []string
	allowedAgentNames  []string
	CanAssertScope     bool
	ExpiresAt          *time.Time
}

func (a *ServiceKeyAdmin) validateIssue(req IssueServiceKeyRequest) (normalizedIssueRequest, error) {
	if a == nil || a.runner == nil || a.now == nil || a.newID == nil {
		return normalizedIssueRequest{}, ErrNilPool
	}
	if err := req.Scope.Validate(); err != nil {
		return normalizedIssueRequest{}, err
	}
	if err := corev2.Ref(req.Name).ValidateName("service_key.name", true); err != nil {
		return normalizedIssueRequest{}, err
	}
	if err := req.ActingServiceKeyID.Validate("acting_service_key_id", false); err != nil {
		return normalizedIssueRequest{}, err
	}
	if err := req.IdempotencyKey.Validate("idempotency_key", true); err != nil {
		return normalizedIssueRequest{}, err
	}
	if err := corev2.ValidateServiceKeyVerifier(req.LookupPrefix, req.SecretHash); err != nil {
		return normalizedIssueRequest{}, err
	}

	operations := make([]string, 0, len(req.Operations))
	seenOperations := make(map[corev2.Operation]struct{}, len(req.Operations))
	for _, operation := range req.Operations {
		switch operation {
		case corev2.OperationConfigApply, corev2.OperationSecretsWrite,
			corev2.OperationKeysManage, corev2.OperationLimitsSync,
			corev2.OperationUsageRead, corev2.OperationAuditRead,
			corev2.OperationConsume, corev2.OperationInvoke:
		default:
			return normalizedIssueRequest{}, fmt.Errorf("%w: unsupported service key operation %q", corev2.ErrInvalidArgument, operation)
		}
		if _, exists := seenOperations[operation]; exists {
			continue
		}
		seenOperations[operation] = struct{}{}
		operations = append(operations, string(operation))
	}
	if len(operations) == 0 {
		return normalizedIssueRequest{}, fmt.Errorf("%w: at least one service key operation is required", corev2.ErrInvalidArgument)
	}
	slices.Sort(operations)

	agentNames := make([]string, 0, len(req.AllowedAgentNames))
	seenAgents := make(map[string]struct{}, len(req.AllowedAgentNames))
	for _, name := range req.AllowedAgentNames {
		if err := name.ValidateName("allowed_agent", true); err != nil {
			return normalizedIssueRequest{}, err
		}
		if _, exists := seenAgents[string(name)]; exists {
			continue
		}
		seenAgents[string(name)] = struct{}{}
		agentNames = append(agentNames, string(name))
	}
	if len(agentNames) > maxServiceKeyAgents {
		return normalizedIssueRequest{}, fmt.Errorf("%w: service key allows at most %d agents", corev2.ErrInvalidArgument, maxServiceKeyAgents)
	}
	slices.Sort(agentNames)
	if (slices.Contains(operations, string(corev2.OperationConsume)) ||
		slices.Contains(operations, string(corev2.OperationInvoke))) && len(agentNames) == 0 {
		return normalizedIssueRequest{}, fmt.Errorf("%w: consume and invoke service keys require an explicit agent allowlist", corev2.ErrInvalidArgument)
	}

	expiresAt := cloneTime(req.ExpiresAt)
	if expiresAt != nil {
		normalized := expiresAt.UTC().Truncate(time.Microsecond)
		expiresAt = &normalized
	}
	normalized := normalizedIssueRequest{
		Scope:              req.Scope,
		ActingServiceKeyID: req.ActingServiceKeyID,
		IdempotencyKey:     req.IdempotencyKey,
		Name:               req.Name,
		LookupPrefix:       req.LookupPrefix,
		operations:         operations,
		allowedAgentNames:  agentNames,
		CanAssertScope:     req.CanAssertScope,
		ExpiresAt:          expiresAt,
	}
	copy(normalized.SecretHash[:], req.SecretHash)
	requestHash, err := serviceKeyIssueHash(normalized)
	if err != nil {
		return normalizedIssueRequest{}, err
	}
	normalized.requestHash = requestHash
	return normalized, nil
}

func lockServiceKeyNamespace(ctx context.Context, db DBTX, scope Scope, requireActive bool) error {
	var id, status string
	err := db.QueryRow(ctx, `
SELECT id, status
FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
FOR UPDATE
`, scope.AccountID, scope.NamespaceID).Scan(&id, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: namespace %q", corev2.ErrInvalidArgument, scope.NamespaceID)
	}
	if err != nil {
		return fmt.Errorf("v2 postgres: lock namespace for service key: %w", err)
	}
	if requireActive && status != "active" {
		return fmt.Errorf("%w: namespace %q is not active", corev2.ErrInvalidArgument, scope.NamespaceID)
	}
	return nil
}

func resolveServiceKeyAgents(ctx context.Context, db DBTX, scope Scope, names []string, requireActive bool) ([]string, error) {
	idsByName := make([]string, 0, len(names))
	for _, name := range names {
		var agentID, status string
		err := db.QueryRow(ctx, `
SELECT id, status
FROM kave_v2.agents
WHERE account_id = $1 AND namespace_id = $2
	  AND name = $3 AND status <> 'archived'
`, scope.AccountID, scope.NamespaceID, name).Scan(&agentID, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: allowed agent %q does not exist in this namespace", corev2.ErrInvalidArgument, name)
		}
		if err != nil {
			return nil, fmt.Errorf("v2 postgres: resolve allowed agent %q: %w", name, err)
		}
		if requireActive && status != "active" {
			return nil, fmt.Errorf("%w: allowed agent %q is not active in this namespace", corev2.ErrInvalidArgument, name)
		}
		idsByName = append(idsByName, agentID)
	}
	slices.Sort(idsByName)
	return idsByName, nil
}

type persistedServiceKey struct {
	id              string
	name            string
	lookupPrefix    string
	secretHash      []byte
	operations      []string
	allowedAgentIDs []string
	canAssertScope  bool
	status          string
	expiresAt       *time.Time
	createdAt       time.Time
}

func loadServiceKeyByName(ctx context.Context, db DBTX, scope Scope, name string) (persistedServiceKey, bool, error) {
	var key persistedServiceKey
	err := db.QueryRow(ctx, `
SELECT id, name, lookup_prefix, secret_hash, capabilities, allowed_agent_ids,
       can_assert_scope, status, expires_at, created_at
FROM kave_v2.service_keys
WHERE account_id = $1 AND namespace_id = $2 AND name = $3
FOR UPDATE
`, scope.AccountID, scope.NamespaceID, name).Scan(
		&key.id, &key.name, &key.lookupPrefix, &key.secretHash, &key.operations, &key.allowedAgentIDs,
		&key.canAssertScope, &key.status, &key.expiresAt, &key.createdAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return persistedServiceKey{}, false, nil
	}
	if err != nil {
		return persistedServiceKey{}, false, fmt.Errorf("v2 postgres: load service key by name: %w", err)
	}
	slices.Sort(key.operations)
	slices.Sort(key.allowedAgentIDs)
	return key, true, nil
}

func (k persistedServiceKey) matches(req normalizedIssueRequest, agentIDs []string) bool {
	return k.name == req.Name &&
		k.lookupPrefix == req.LookupPrefix &&
		len(k.secretHash) == sha256.Size && subtle.ConstantTimeCompare(k.secretHash, req.SecretHash[:]) == 1 &&
		slices.Equal(k.operations, req.operations) &&
		slices.Equal(k.allowedAgentIDs, agentIDs) &&
		k.canAssertScope == req.CanAssertScope &&
		equalTime(k.expiresAt, req.ExpiresAt)
}

func (k persistedServiceKey) result(created bool) IssuedServiceKey {
	return IssuedServiceKey{
		ID:              k.id,
		Name:            k.name,
		Prefix:          RawServiceKeyPrefix + k.lookupPrefix,
		Operations:      stringsToOperations(k.operations),
		AllowedAgentIDs: slices.Clone(k.allowedAgentIDs),
		CanAssertScope:  k.canAssertScope,
		ExpiresAt:       cloneTime(k.expiresAt),
		CreatedAt:       k.createdAt,
		Status:          k.status,
		Created:         created,
	}
}

type serviceKeyIssueAuditDetails struct {
	RequestHash string           `json:"request_hash"`
	Result      IssuedServiceKey `json:"result"`
}

func serviceKeyIssueHash(req normalizedIssueRequest) (string, error) {
	payload, err := json.Marshal(struct {
		Name              string     `json:"name"`
		LookupPrefix      string     `json:"lookup_prefix"`
		SecretHash        string     `json:"secret_hash"`
		Operations        []string   `json:"operations"`
		AllowedAgentNames []string   `json:"allowed_agent_names"`
		CanAssertScope    bool       `json:"can_assert_scope"`
		ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	}{req.Name, req.LookupPrefix, hex.EncodeToString(req.SecretHash[:]), req.operations, req.allowedAgentNames, req.CanAssertScope, req.ExpiresAt})
	if err != nil {
		return "", fmt.Errorf("v2 postgres: encode service-key issue request: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func serviceKeyIssueAuditID(scope Scope, key corev2.Ref) string {
	digest := sha256.Sum256([]byte("service_key.issue\x00" + scope.AccountID + "\x00" + scope.NamespaceID + "\x00" + string(key)))
	return "aud_key_issue_" + hex.EncodeToString(digest[:16])
}

func loadPriorServiceKeyIssue(ctx context.Context, db DBTX, req normalizedIssueRequest) (IssuedServiceKey, bool, error) {
	var raw []byte
	err := db.QueryRow(ctx, `
SELECT details
FROM kave_v2.audit_events
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
  AND event = 'service_key.issued'
`, req.Scope.AccountID, req.Scope.NamespaceID,
		serviceKeyIssueAuditID(req.Scope, req.IdempotencyKey)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return IssuedServiceKey{}, false, nil
	}
	if err != nil {
		return IssuedServiceKey{}, false, fmt.Errorf("v2 postgres: load service-key issue idempotency: %w", err)
	}
	var details serviceKeyIssueAuditDetails
	if err := json.Unmarshal(raw, &details); err != nil || details.RequestHash == "" || details.Result.ID == "" {
		return IssuedServiceKey{}, false, errors.New("v2 postgres: corrupt service-key issue idempotency record")
	}
	if details.RequestHash != req.requestHash {
		return IssuedServiceKey{}, false, &corev2.IdempotencyConflictError{Key: req.IdempotencyKey}
	}
	persisted, found, err := loadServiceKeyByName(ctx, db, req.Scope, details.Result.Name)
	if err != nil {
		return IssuedServiceKey{}, false, err
	}
	if !found || persisted.id != details.Result.ID {
		return IssuedServiceKey{}, false, errors.New("v2 postgres: service-key issue record references a missing key")
	}
	return persisted.result(false), true, nil
}

func appendServiceKeyIssueAudit(ctx context.Context, db DBTX, req normalizedIssueRequest, result IssuedServiceKey, now time.Time) error {
	persisted := result
	// The lookup prefix is not a verifier, but keeping it out of the immutable
	// audit stream reduces pre-authentication identifier exposure. Replays load
	// it from the RLS-scoped service_keys row.
	persisted.Prefix = ""
	details, err := json.Marshal(serviceKeyIssueAuditDetails{RequestHash: req.requestHash, Result: persisted})
	if err != nil {
		return fmt.Errorf("v2 postgres: encode service-key issue audit: %w", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.audit_events (
    id, account_id, namespace_id, service_key_id,
    event, resource_type, resource_id, outcome, request_id, details, created_at
) VALUES ($1, $2, $3, NULLIF($4, ''),
          'service_key.issued', 'service_key', $5, 'succeeded', $6, $7, $8)
`, serviceKeyIssueAuditID(req.Scope, req.IdempotencyKey), req.Scope.AccountID,
		req.Scope.NamespaceID, req.ActingServiceKeyID, result.ID,
		req.IdempotencyKey, details, now); err != nil {
		return fmt.Errorf("v2 postgres: append service-key issue audit: %w", err)
	}
	return nil
}

func appendServiceKeyAudit(ctx context.Context, db DBTX, scope Scope, actingServiceKeyID corev2.Ref, event, resourceID, name string, detail map[string]any, now time.Time) error {
	details, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("v2 postgres: encode service key audit: %w", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.audit_events (
    id, account_id, namespace_id, service_key_id,
    event, resource_type, resource_id, outcome, details, created_at
) VALUES ($1, $2, $3, NULLIF($4, ''), $5, 'service_key', $6, 'succeeded', $7, $8)
`, ids.New("aud"), scope.AccountID, scope.NamespaceID,
		actingServiceKeyID, event, resourceID, details, now); err != nil {
		return fmt.Errorf("v2 postgres: append service key audit for %q: %w", name, err)
	}
	return nil
}

func stringsToOperations(values []string) []corev2.Operation {
	result := make([]corev2.Operation, len(values))
	for i, value := range values {
		result[i] = corev2.Operation(value)
	}
	return result
}

func equalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
