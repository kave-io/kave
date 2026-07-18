package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/pkg/ids"
	corev2 "github.com/kave-io/kave/core/v2"
)

const maxApplyAttempts = 8

// ApplyStore owns transactional, declarative namespace configuration. It is
// deliberately separate from the admission store even though both serialize
// through the same namespace row.
type ApplyStore struct {
	runner *ScopedRunner
}

func NewApplyStore(pool *pgxpool.Pool) (*ApplyStore, error) {
	runner, err := NewScopedRunner(pool)
	if err != nil {
		return nil, err
	}
	return &ApplyStore{runner: runner}, nil
}

func (s *ApplyStore) Apply(ctx context.Context, req corev2.ApplyRequest) (corev2.ApplyResult, error) {
	if err := req.Validate(); err != nil {
		return corev2.ApplyResult{}, err
	}
	requestHash, err := req.Hash()
	if err != nil {
		return corev2.ApplyResult{}, err
	}
	manifest := req.CanonicalManifest()
	namespaceID := deterministicNamespaceID(manifest.Namespace)
	initialNamespaceID := namespaceID
	if !req.Caller.Bootstrap {
		// A standing config key is namespace-scoped. Start RLS at its authenticated
		// namespace and require the manifest tuple to match that exact row; only
		// the account-scoped offline bootstrap may discover a legacy ID by tuple.
		initialNamespaceID = req.Caller.NamespaceID
	}

	for attempt := 1; attempt <= maxApplyAttempts; attempt++ {
		var result corev2.ApplyResult
		err = s.runner.WithScope(ctx, Scope{
			AccountID:   string(req.Caller.AccountID),
			NamespaceID: string(initialNamespaceID),
		}, func(txCtx context.Context, db DBTX) error {
			var txErr error
			result, txErr = applyTx(txCtx, db, req, manifest, requestHash, namespaceID)
			return txErr
		})
		var dryRun *dryRunRollback
		if errors.As(err, &dryRun) {
			return dryRun.result, nil
		}
		if err == nil {
			return result, nil
		}
		if !isRetryableTransactionError(err) || attempt == maxApplyAttempts {
			return corev2.ApplyResult{}, err
		}
	}
	return corev2.ApplyResult{}, err
}

type dryRunRollback struct{ result corev2.ApplyResult }

func (e *dryRunRollback) Error() string { return "v2 postgres: roll back apply dry-run" }

func applyTx(
	ctx context.Context,
	db DBTX,
	req corev2.ApplyRequest,
	manifest corev2.Manifest,
	requestHash string,
	desiredNamespaceID corev2.Ref,
) (corev2.ApplyResult, error) {
	if req.Caller.Bootstrap {
		if err := installApplyNamespaceLookup(ctx, db, manifest.Namespace); err != nil {
			return corev2.ApplyResult{}, err
		}
	}
	namespaceID, revision, created, err := resolveApplyNamespace(ctx, db, req, manifest.Namespace, desiredNamespaceID)
	if err != nil {
		return corev2.ApplyResult{}, err
	}
	if req.Caller.NamespaceID != "" && req.Caller.NamespaceID != namespaceID {
		return corev2.ApplyResult{}, fmt.Errorf("%w: service key cannot apply another namespace", corev2.ErrUnauthorized)
	}

	// Namespace children are RLS-bound. Existing databases may contain a
	// non-deterministic V2 namespace ID, so replace the initial candidate scope
	// with the ID resolved by the account-scoped namespace lookup.
	var installed string
	if err := db.QueryRow(ctx, `SELECT set_config('kave.namespace_id', $1, true)`, namespaceID).Scan(&installed); err != nil {
		return corev2.ApplyResult{}, fmt.Errorf("v2 postgres: install resolved namespace: %w", err)
	}
	if installed != string(namespaceID) {
		return corev2.ApplyResult{}, errors.New("v2 postgres: database returned unexpected namespace scope")
	}
	if req.Caller.Bootstrap {
		if err := clearApplyNamespaceLookup(ctx, db); err != nil {
			return corev2.ApplyResult{}, err
		}
	}

	// Admission holds FOR SHARE on this exact row. Apply and future SyncLimits
	// hold FOR UPDATE, so a request sees either the old or new complete config.
	if err := db.QueryRow(ctx, `
SELECT revision
FROM kave_v2.namespaces
WHERE account_id = $1 AND id = $2
FOR UPDATE
`, req.Caller.AccountID, namespaceID).Scan(&revision); err != nil {
		return corev2.ApplyResult{}, fmt.Errorf("v2 postgres: lock namespace: %w", err)
	}

	if !req.DryRun {
		prior, found, err := loadPriorApply(ctx, db, namespaceID, req.IdempotencyKey, requestHash)
		if err != nil {
			return corev2.ApplyResult{}, err
		}
		if found {
			return prior, nil
		}
	}
	if req.ExpectedRevision > 0 && req.ExpectedRevision != revision {
		return corev2.ApplyResult{}, &corev2.RevisionConflictError{Expected: req.ExpectedRevision, Actual: revision}
	}

	state, err := loadApplyState(ctx, db, req.Caller.AccountID, namespaceID)
	if err != nil {
		return corev2.ApplyResult{}, err
	}
	changes := make([]corev2.Change, 0, 1+len(manifest.Routes)+len(manifest.Agents)+len(manifest.Limits))
	if created {
		changes = append(changes, corev2.Change{Kind: corev2.ChangeCreate, ResourceKind: "namespace", Name: manifest.Namespace.Application})
	} else {
		changes = append(changes, corev2.Change{Kind: corev2.ChangeUnchanged, ResourceKind: "namespace", Name: manifest.Namespace.Application})
	}

	changed, err := reconcileRoutes(ctx, db, req.Caller.AccountID, namespaceID, manifest.Routes, state, &changes, req.DryRun)
	if err != nil {
		return corev2.ApplyResult{}, err
	}
	configChanged := created || changed
	changed, err = reconcileAgents(ctx, db, req.Caller.AccountID, namespaceID, manifest.Agents, state, &changes, req.DryRun)
	if err != nil {
		return corev2.ApplyResult{}, err
	}
	configChanged = configChanged || changed
	changed, err = reconcileLimits(ctx, db, req.Caller.AccountID, namespaceID, manifest.Limits, state, &changes, req.DryRun)
	if err != nil {
		return corev2.ApplyResult{}, err
	}
	configChanged = configChanged || changed

	if req.Prune {
		changed, err = pruneApplyState(ctx, db, req.Caller.AccountID, namespaceID, manifest, state, &changes, req.DryRun)
		if err != nil {
			return corev2.ApplyResult{}, err
		}
		configChanged = configChanged || changed
	}

	sortChanges(changes)
	nextRevision := revision
	if configChanged && !created {
		nextRevision++
		if !req.DryRun {
			if _, err := db.Exec(ctx, `
UPDATE kave_v2.namespaces
SET revision = $3
WHERE account_id = $1 AND id = $2
`, req.Caller.AccountID, namespaceID, nextRevision); err != nil {
				return corev2.ApplyResult{}, fmt.Errorf("v2 postgres: update namespace revision: %w", err)
			}
		}
	}
	result := corev2.ApplyResult{
		NamespaceID: namespaceID,
		Revision:    nextRevision,
		Applied:     !req.DryRun,
		Changes:     changes,
	}
	if req.DryRun {
		return corev2.ApplyResult{}, &dryRunRollback{result: result}
	}
	if err := appendApplyAudit(ctx, db, req, requestHash, result); err != nil {
		return corev2.ApplyResult{}, err
	}
	return result, nil
}

func installApplyNamespaceLookup(ctx context.Context, db DBTX, ns corev2.Namespace) error {
	var application, environment string
	if err := db.QueryRow(ctx, `
SELECT
    set_config('kave.apply_application', $1, true),
    set_config('kave.apply_environment', $2, true)
`, ns.Application, ns.Environment).Scan(&application, &environment); err != nil {
		return fmt.Errorf("v2 postgres: install namespace tuple lookup: %w", err)
	}
	if application != string(ns.Application) || environment != string(ns.Environment) {
		return errors.New("v2 postgres: database returned unexpected namespace tuple lookup")
	}
	return nil
}

func clearApplyNamespaceLookup(ctx context.Context, db DBTX) error {
	var application, environment string
	if err := db.QueryRow(ctx, `
SELECT
    set_config('kave.apply_application', '', true),
    set_config('kave.apply_environment', '', true)
`).Scan(&application, &environment); err != nil {
		return fmt.Errorf("v2 postgres: clear namespace tuple lookup: %w", err)
	}
	if application != "" || environment != "" {
		return errors.New("v2 postgres: database did not clear namespace tuple lookup")
	}
	return nil
}

func resolveApplyNamespace(
	ctx context.Context,
	db DBTX,
	req corev2.ApplyRequest,
	ns corev2.Namespace,
	desiredID corev2.Ref,
) (corev2.Ref, int64, bool, error) {
	var id, status string
	var revision int64
	query := `
SELECT id, revision, status
FROM kave_v2.namespaces
WHERE account_id = $1 AND application = $2 AND environment = $3
FOR UPDATE
`
	args := []any{ns.Account, ns.Application, ns.Environment}
	if !req.Caller.Bootstrap {
		query = `
SELECT id, revision, status
FROM kave_v2.namespaces
WHERE account_id = $1 AND application = $2 AND environment = $3 AND id = $4
FOR UPDATE
`
		args = append(args, req.Caller.NamespaceID)
	}
	err := db.QueryRow(ctx, query, args...).Scan(&id, &revision, &status)
	if err == nil {
		// Disabled is an administrative kill switch, not declarative drift.
		// Neither an in-flight control key nor the offline bootstrap path may
		// turn a disabled namespace back on through an ordinary Apply.
		if status != "active" {
			return "", 0, false, fmt.Errorf("%w: namespace is not active", corev2.ErrUnauthorized)
		}
		return corev2.Ref(id), revision, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, fmt.Errorf("v2 postgres: resolve namespace: %w", err)
	}
	if !req.Caller.Bootstrap {
		return "", 0, false, fmt.Errorf("%w: manifest does not match caller namespace", corev2.ErrUnauthorized)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.namespaces (id, account_id, application, environment)
VALUES ($1, $2, $3, $4)
`, desiredID, ns.Account, ns.Application, ns.Environment); err != nil {
		return "", 0, false, fmt.Errorf("v2 postgres: create namespace: %w", err)
	}
	return desiredID, 1, true, nil
}

type applyAuditDetails struct {
	RequestHash string             `json:"request_hash"`
	Result      corev2.ApplyResult `json:"result"`
}

func loadPriorApply(ctx context.Context, db DBTX, namespaceID corev2.Ref, key corev2.Ref, requestHash string) (corev2.ApplyResult, bool, error) {
	var raw []byte
	err := db.QueryRow(ctx, `
SELECT details
FROM kave_v2.audit_events
WHERE id = $1 AND event = 'config.apply'
`, applyAuditID(namespaceID, key)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return corev2.ApplyResult{}, false, nil
	}
	if err != nil {
		return corev2.ApplyResult{}, false, fmt.Errorf("v2 postgres: load prior apply: %w", err)
	}
	var details applyAuditDetails
	if err := json.Unmarshal(raw, &details); err != nil {
		return corev2.ApplyResult{}, false, fmt.Errorf("v2 postgres: decode prior apply: %w", err)
	}
	if details.RequestHash != requestHash {
		return corev2.ApplyResult{}, false, &corev2.IdempotencyConflictError{Key: key}
	}
	if details.Result.NamespaceID == "" || details.Result.Revision < 1 || !details.Result.Applied {
		return corev2.ApplyResult{}, false, errors.New("v2 postgres: prior apply has an invalid result")
	}
	return details.Result, true, nil
}

func appendApplyAudit(ctx context.Context, db DBTX, req corev2.ApplyRequest, requestHash string, result corev2.ApplyResult) error {
	details, err := json.Marshal(applyAuditDetails{RequestHash: requestHash, Result: result})
	if err != nil {
		return fmt.Errorf("v2 postgres: encode apply audit: %w", err)
	}
	serviceKeyID := req.Caller.ServiceKeyID
	if req.Caller.Bootstrap {
		// The bootstrap credential is configured out-of-band and never has a
		// service_keys row. Preserve the audit event while leaving its optional
		// service-key foreign key NULL.
		serviceKeyID = ""
	}
	if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.audit_events (
    id, account_id, namespace_id, service_key_id,
    event, resource_type, resource_id, outcome, request_id, details
) VALUES (
    $1, $2, $3, NULLIF($4, ''),
    'config.apply', 'namespace', $3, 'succeeded', $5, $6
)
`, applyAuditID(result.NamespaceID, req.IdempotencyKey), req.Caller.AccountID,
		result.NamespaceID, serviceKeyID, req.IdempotencyKey, details); err != nil {
		return fmt.Errorf("v2 postgres: append apply audit: %w", err)
	}
	return nil
}

func deterministicNamespaceID(ns corev2.Namespace) corev2.Ref {
	sum := sha256.Sum256([]byte(string(ns.Account) + "\x00" + string(ns.Application) + "\x00" + string(ns.Environment)))
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
	return corev2.Ref("nsp_" + encoded[:26])
}

func applyAuditID(namespaceID, key corev2.Ref) string {
	sum := sha256.Sum256([]byte(string(namespaceID) + "\x00" + string(key)))
	return "aud_apply_" + fmt.Sprintf("%x", sum[:])
}

type routeRow struct {
	id, name, provider, baseURL, secretID, secretName, defaultModel, status string
	allowedModels                                                           []string
	pricing                                                                 []corev2.ModelPrice
	pricingRevision, revision                                               int64
}

type agentRow struct {
	id, name, kind, routeID, routeName, status string
	revision                                   int64
}

type limitRow struct {
	id, key, source, metric, tenant, actor, billTo, agentID, agentName, model, feature, window string
	hardCap, generation, revision                                                              int64
	softCap                                                                                    *int64
	enabled, superseded                                                                        bool
}

type secretRow struct {
	id      string
	backend string
}

type applyState struct {
	secrets map[string]secretRow
	routes  map[string]routeRow
	agents  map[string]agentRow
	limits  map[string]limitRow
}

func loadApplyState(ctx context.Context, db DBTX, accountID, namespaceID corev2.Ref) (*applyState, error) {
	state := &applyState{
		secrets: map[string]secretRow{}, routes: map[string]routeRow{},
		agents: map[string]agentRow{}, limits: map[string]limitRow{},
	}
	rows, err := db.Query(ctx, `
SELECT name, id, backend FROM kave_v2.secrets
WHERE account_id = $1 AND namespace_id = $2 AND status = 'active'
`, accountID, namespaceID)
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: list active secrets: %w", err)
	}
	for rows.Next() {
		var name string
		var secret secretRow
		if err := rows.Scan(&name, &secret.id, &secret.backend); err != nil {
			rows.Close()
			return nil, fmt.Errorf("v2 postgres: scan secret: %w", err)
		}
		state.secrets[name] = secret
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("v2 postgres: iterate secrets: %w", err)
	}
	rows.Close()

	rows, err = db.Query(ctx, `
SELECT r.id, r.name, r.provider, r.base_url, COALESCE(r.secret_id, ''),
       COALESCE(s.name, ''), r.model_policy, r.pricing_revision, r.pricing,
       r.status, r.revision
FROM kave_v2.provider_routes r
LEFT JOIN kave_v2.secrets s
  ON s.account_id = r.account_id AND s.namespace_id = r.namespace_id AND s.id = r.secret_id
WHERE r.account_id = $1 AND r.namespace_id = $2
`, accountID, namespaceID)
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: list routes: %w", err)
	}
	for rows.Next() {
		var row routeRow
		var policyBytes, pricingBytes []byte
		if err := rows.Scan(&row.id, &row.name, &row.provider, &row.baseURL, &row.secretID,
			&row.secretName, &policyBytes, &row.pricingRevision, &pricingBytes,
			&row.status, &row.revision); err != nil {
			rows.Close()
			return nil, fmt.Errorf("v2 postgres: scan route: %w", err)
		}
		var policy routeModelPolicy
		if err := json.Unmarshal(policyBytes, &policy); err != nil {
			rows.Close()
			return nil, fmt.Errorf("v2 postgres: decode route %q policy: %w", row.name, err)
		}
		row.allowedModels, row.defaultModel = policy.AllowedModels, policy.DefaultModel
		var pricing routePriceDocument
		if err := json.Unmarshal(pricingBytes, &pricing); err != nil {
			rows.Close()
			return nil, fmt.Errorf("v2 postgres: decode route %q pricing: %w", row.name, err)
		}
		row.pricing = modelPricesFromDocument(pricing)
		state.routes[row.name] = row
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("v2 postgres: iterate routes: %w", err)
	}
	rows.Close()

	rows, err = db.Query(ctx, `
SELECT a.id, a.name, a.kind, a.route_id, r.name, a.status, a.revision
FROM kave_v2.agents a
JOIN kave_v2.provider_routes r
  ON r.account_id = a.account_id AND r.namespace_id = a.namespace_id AND r.id = a.route_id
WHERE a.account_id = $1 AND a.namespace_id = $2
  AND a.status <> 'archived'
`, accountID, namespaceID)
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: list agents: %w", err)
	}
	for rows.Next() {
		var row agentRow
		if err := rows.Scan(&row.id, &row.name, &row.kind, &row.routeID, &row.routeName, &row.status, &row.revision); err != nil {
			rows.Close()
			return nil, fmt.Errorf("v2 postgres: scan agent: %w", err)
		}
		state.agents[row.name] = row
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("v2 postgres: iterate agents: %w", err)
	}
	rows.Close()

	rows, err = db.Query(ctx, `
SELECT l.id, l.external_key, l.generation, l.source, l.metric,
       COALESCE(l.tenant_ref, ''), COALESCE(l.actor_ref, ''), COALESCE(l.billing_ref, ''),
       COALESCE(l.agent_id, ''), COALESCE(a.name, ''), COALESCE(l.model, ''), COALESCE(l.feature, ''),
       l.window_kind, l.hard_cap, l.soft_cap, l.enabled,
       l.superseded_at IS NOT NULL, l.revision
FROM kave_v2.limits l
LEFT JOIN kave_v2.agents a
  ON a.account_id = l.account_id AND a.namespace_id = l.namespace_id AND a.id = l.agent_id
WHERE l.account_id = $1 AND l.namespace_id = $2
ORDER BY l.external_key, l.generation
`, accountID, namespaceID)
	if err != nil {
		return nil, fmt.Errorf("v2 postgres: list limits: %w", err)
	}
	for rows.Next() {
		var row limitRow
		var soft sql.NullInt64
		if err := rows.Scan(&row.id, &row.key, &row.generation, &row.source, &row.metric,
			&row.tenant, &row.actor, &row.billTo, &row.agentID, &row.agentName,
			&row.model, &row.feature, &row.window, &row.hardCap, &soft, &row.enabled,
			&row.superseded, &row.revision); err != nil {
			rows.Close()
			return nil, fmt.Errorf("v2 postgres: scan limit: %w", err)
		}
		if soft.Valid {
			value := soft.Int64
			row.softCap = &value
		}
		state.limits[row.key] = row
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("v2 postgres: iterate limits: %w", err)
	}
	rows.Close()
	return state, nil
}

type routeModelPolicy struct {
	AllowedModels []string `json:"allowed_models,omitempty"`
	DefaultModel  string   `json:"default_model,omitempty"`
}

func reconcileRoutes(ctx context.Context, db DBTX, accountID, namespaceID corev2.Ref, specs []corev2.RouteSpec, state *applyState, changes *[]corev2.Change, dryRun bool) (bool, error) {
	changed := false
	for _, spec := range specs {
		secret, exists := state.secrets[string(spec.Secret)]
		if !exists {
			return false, fmt.Errorf("%w: route %q references missing or inactive secret %q", corev2.ErrInvalidArgument, spec.Name, spec.Secret)
		}
		if secret.backend != "encrypted" {
			return false, fmt.Errorf("%w: route %q requires an encrypted provider secret; secret %q uses unsupported backend %q", corev2.ErrInvalidArgument, spec.Name, spec.Secret, secret.backend)
		}
		secretID := secret.id
		baseURL, err := routeBaseURL(spec)
		if err != nil {
			return false, err
		}
		pricingRevision := spec.PricingRevision
		if pricingRevision == 0 {
			pricingRevision = 1
		}
		policy := routeModelPolicy{AllowedModels: spec.AllowedModels, DefaultModel: spec.DefaultModel}
		policyBytes, err := json.Marshal(policy)
		if err != nil {
			return false, fmt.Errorf("v2 postgres: encode route %q policy: %w", spec.Name, err)
		}
		pricing := routePricingDocument(spec.Pricing)
		pricingBytes, err := json.Marshal(pricing)
		if err != nil {
			return false, fmt.Errorf("v2 postgres: encode route %q pricing: %w", spec.Name, err)
		}
		current, exists := state.routes[string(spec.Name)]
		if !exists {
			changed = true
			*changes = append(*changes, corev2.Change{Kind: corev2.ChangeCreate, ResourceKind: "route", Name: spec.Name})
			id := ids.New("rte")
			state.routes[string(spec.Name)] = routeRow{id: id, name: string(spec.Name), provider: string(spec.Provider), baseURL: baseURL, secretID: secretID, secretName: string(spec.Secret), allowedModels: spec.AllowedModels, defaultModel: spec.DefaultModel, pricing: spec.Pricing, pricingRevision: pricingRevision, status: "active", revision: 1}
			if !dryRun {
				if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.provider_routes (
    id, account_id, namespace_id, name, provider, base_url, secret_id,
    model_policy, pricing_revision, pricing, status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'active')
`, id, accountID, namespaceID, spec.Name, spec.Provider, baseURL, secretID, policyBytes, pricingRevision, pricingBytes); err != nil {
					return false, fmt.Errorf("v2 postgres: create route %q: %w", spec.Name, err)
				}
			}
			continue
		}
		if err := validateRoutePricingRevision(current, spec, pricingRevision); err != nil {
			return false, err
		}
		fields := changedRouteFields(current, spec, baseURL, secretID, pricingRevision)
		if len(fields) == 0 {
			*changes = append(*changes, corev2.Change{Kind: corev2.ChangeUnchanged, ResourceKind: "route", Name: spec.Name})
			continue
		}
		changed = true
		*changes = append(*changes, corev2.Change{Kind: corev2.ChangeUpdate, ResourceKind: "route", Name: spec.Name, Fields: fields})
		current.provider, current.baseURL, current.secretID, current.secretName = string(spec.Provider), baseURL, secretID, string(spec.Secret)
		current.allowedModels, current.defaultModel, current.pricing, current.pricingRevision, current.status = spec.AllowedModels, spec.DefaultModel, spec.Pricing, pricingRevision, "active"
		state.routes[string(spec.Name)] = current
		if !dryRun {
			if _, err := db.Exec(ctx, `
UPDATE kave_v2.provider_routes
SET provider = $4, base_url = $5, secret_id = $6, model_policy = $7,
    pricing_revision = $8, pricing = $9, status = 'active', revision = revision + 1
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, accountID, namespaceID, current.id, spec.Provider, baseURL, secretID, policyBytes, pricingRevision, pricingBytes); err != nil {
				return false, fmt.Errorf("v2 postgres: update route %q: %w", spec.Name, err)
			}
		}
	}
	return changed, nil
}

func validateRoutePricingRevision(current routeRow, spec corev2.RouteSpec, desiredRevision int64) error {
	if desiredRevision < current.pricingRevision {
		return fmt.Errorf("%w: route %q pricing revision cannot decrease below %d", corev2.ErrRevisionConflict, spec.Name, current.pricingRevision)
	}
	if !slices.Equal(current.pricing, spec.Pricing) && desiredRevision <= current.pricingRevision {
		return fmt.Errorf("%w: route %q pricing revision must increase beyond %d when pricing changes", corev2.ErrRevisionConflict, spec.Name, current.pricingRevision)
	}
	return nil
}

func changedRouteFields(current routeRow, spec corev2.RouteSpec, baseURL, secretID string, pricingRevision int64) []string {
	fields := make([]string, 0, 8)
	if current.provider != string(spec.Provider) {
		fields = append(fields, "provider")
	}
	if current.baseURL != baseURL {
		fields = append(fields, "base_url")
	}
	if current.secretID != secretID {
		fields = append(fields, "secret")
	}
	if !slices.Equal(current.allowedModels, spec.AllowedModels) {
		fields = append(fields, "allowed_models")
	}
	if current.defaultModel != spec.DefaultModel {
		fields = append(fields, "default_model")
	}
	if current.pricingRevision != pricingRevision {
		fields = append(fields, "pricing_revision")
	}
	if !slices.Equal(current.pricing, spec.Pricing) {
		fields = append(fields, "pricing")
	}
	if current.status != "active" {
		fields = append(fields, "status")
	}
	slices.Sort(fields)
	return fields
}

func routePricingDocument(prices []corev2.ModelPrice) routePriceDocument {
	models := make(map[string]routeModelPrice, len(prices))
	for _, price := range prices {
		models[string(price.Model)] = routeModelPrice{
			InputNanosPerMillionTokens:  price.InputNanosPerMillionTokens,
			OutputNanosPerMillionTokens: price.OutputNanosPerMillionTokens,
		}
	}
	return routePriceDocument{Models: models}
}

func modelPricesFromDocument(document routePriceDocument) []corev2.ModelPrice {
	prices := make([]corev2.ModelPrice, 0, len(document.Models))
	for model, price := range document.Models {
		prices = append(prices, corev2.ModelPrice{
			Model:                       corev2.Ref(model),
			InputNanosPerMillionTokens:  price.InputNanosPerMillionTokens,
			OutputNanosPerMillionTokens: price.OutputNanosPerMillionTokens,
		})
	}
	slices.SortFunc(prices, func(a, b corev2.ModelPrice) int {
		return strings.Compare(string(a.Model), string(b.Model))
	})
	return prices
}

func routeBaseURL(spec corev2.RouteSpec) (string, error) {
	if spec.BaseURL != "" {
		return spec.BaseURL, nil
	}
	if strings.EqualFold(string(spec.Provider), "openai") {
		return "https://api.openai.com/v1", nil
	}
	return "", fmt.Errorf("%w: route %q requires base_url for provider %q", corev2.ErrInvalidArgument, spec.Name, spec.Provider)
}

func reconcileAgents(ctx context.Context, db DBTX, accountID, namespaceID corev2.Ref, specs []corev2.AgentSpec, state *applyState, changes *[]corev2.Change, dryRun bool) (bool, error) {
	changed := false
	for _, spec := range specs {
		route, exists := state.routes[string(spec.Route)]
		if !exists {
			return false, fmt.Errorf("%w: agent %q references missing route %q", corev2.ErrInvalidArgument, spec.Name, spec.Route)
		}
		status := "disabled"
		if spec.Enabled {
			status = "active"
		}
		current, exists := state.agents[string(spec.Name)]
		if !exists {
			changed = true
			*changes = append(*changes, corev2.Change{Kind: corev2.ChangeCreate, ResourceKind: "agent", Name: spec.Name})
			id := ids.New("agt")
			state.agents[string(spec.Name)] = agentRow{id: id, name: string(spec.Name), kind: string(spec.Kind), routeID: route.id, routeName: string(spec.Route), status: status, revision: 1}
			if !dryRun {
				if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.agents (
    id, account_id, namespace_id, name, kind, route_id, status
) VALUES ($1, $2, $3, $4, $5, $6, $7)
`, id, accountID, namespaceID, spec.Name, spec.Kind, route.id, status); err != nil {
					return false, fmt.Errorf("v2 postgres: create agent %q: %w", spec.Name, err)
				}
			}
			continue
		}
		fields := make([]string, 0, 3)
		if current.kind != string(spec.Kind) {
			fields = append(fields, "kind")
		}
		if current.routeID != route.id {
			fields = append(fields, "route")
		}
		if current.status != status {
			fields = append(fields, "status")
		}
		if len(fields) == 0 {
			*changes = append(*changes, corev2.Change{Kind: corev2.ChangeUnchanged, ResourceKind: "agent", Name: spec.Name})
			continue
		}
		changed = true
		slices.Sort(fields)
		*changes = append(*changes, corev2.Change{Kind: corev2.ChangeUpdate, ResourceKind: "agent", Name: spec.Name, Fields: fields})
		current.kind, current.routeID, current.routeName, current.status = string(spec.Kind), route.id, string(spec.Route), status
		state.agents[string(spec.Name)] = current
		if !dryRun {
			if _, err := db.Exec(ctx, `
UPDATE kave_v2.agents
SET kind = $4, route_id = $5, status = $6, revision = revision + 1
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, accountID, namespaceID, current.id, spec.Kind, route.id, status); err != nil {
				return false, fmt.Errorf("v2 postgres: update agent %q: %w", spec.Name, err)
			}
		}
	}
	return changed, nil
}

func reconcileLimits(ctx context.Context, db DBTX, accountID, namespaceID corev2.Ref, specs []corev2.LimitSpec, state *applyState, changes *[]corev2.Change, dryRun bool) (bool, error) {
	changed := false
	for _, spec := range specs {
		agentID := ""
		if spec.Selector.Agent != "" {
			agentID = state.agents[string(spec.Selector.Agent)].id
		}
		window := limitWindowKind(spec.Window)
		current, exists := state.limits[string(spec.Key)]
		if exists && !current.superseded && current.source != "operator" {
			return false, fmt.Errorf("%w: limit key %q is owned by %q, not declarative Apply", corev2.ErrInvalidArgument, spec.Key, current.source)
		}
		if !exists || current.superseded {
			changed = true
			*changes = append(*changes, corev2.Change{Kind: corev2.ChangeCreate, ResourceKind: "limit", Name: spec.Key})
			id := ids.New("lim")
			generation := int64(1)
			if exists {
				generation = current.generation + 1
			}
			state.limits[string(spec.Key)] = limitRow{id: id, key: string(spec.Key), generation: generation, source: "operator", metric: string(spec.Metric), tenant: string(spec.Selector.Tenant), actor: string(spec.Selector.Actor), billTo: string(spec.Selector.BillTo), agentID: agentID, agentName: string(spec.Selector.Agent), model: string(spec.Selector.Model), feature: string(spec.Selector.Feature), window: window, hardCap: spec.HardCap, softCap: cloneInt64(spec.SoftCap), enabled: spec.Enabled, revision: 1}
			if !dryRun {
				if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.limits (
    id, account_id, namespace_id, external_key, generation, source, metric,
    tenant_ref, actor_ref, billing_ref, agent_id, model, feature,
    hard_cap, soft_cap, window_kind, enabled
) VALUES (
    $1, $2, $3, $4, $5, 'operator', $6,
    NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''),
    $13, $14, $15, $16
)
`, id, accountID, namespaceID, spec.Key, generation, spec.Metric,
					spec.Selector.Tenant, spec.Selector.Actor, spec.Selector.BillTo, agentID,
					spec.Selector.Model, spec.Selector.Feature, spec.HardCap, spec.SoftCap, window, spec.Enabled); err != nil {
					return false, fmt.Errorf("v2 postgres: create limit %q: %w", spec.Key, err)
				}
			}
			continue
		}
		fields := changedLimitFields(current, spec, agentID, window)
		if len(fields) == 0 {
			*changes = append(*changes, corev2.Change{Kind: corev2.ChangeUnchanged, ResourceKind: "limit", Name: spec.Key})
			continue
		}
		changed = true
		*changes = append(*changes, corev2.Change{Kind: corev2.ChangeUpdate, ResourceKind: "limit", Name: spec.Key, Fields: fields})
		if limitPolicyOnlyChange(fields) {
			current.hardCap = spec.HardCap
			current.softCap = cloneInt64(spec.SoftCap)
			current.enabled = spec.Enabled
			current.revision++
			state.limits[string(spec.Key)] = current
			if !dryRun {
				if err := updateCurrentLimitPolicy(ctx, db, accountID, namespaceID, current.id, spec, nil); err != nil {
					return false, fmt.Errorf("v2 postgres: update limit policy %q: %w", spec.Key, err)
				}
			}
			continue
		}
		newID := ids.New("lim")
		next := limitRow{id: newID, key: current.key, generation: current.generation + 1, source: current.source,
			metric: string(spec.Metric), tenant: string(spec.Selector.Tenant), actor: string(spec.Selector.Actor),
			billTo: string(spec.Selector.BillTo), agentID: agentID, agentName: string(spec.Selector.Agent),
			model: string(spec.Selector.Model), feature: string(spec.Selector.Feature), window: window,
			hardCap: spec.HardCap, softCap: cloneInt64(spec.SoftCap), enabled: spec.Enabled, revision: 1}
		state.limits[string(spec.Key)] = next
		if !dryRun {
			if _, err := db.Exec(ctx, `
UPDATE kave_v2.limits
SET enabled = FALSE, superseded_at = transaction_timestamp()
WHERE account_id = $1 AND namespace_id = $2 AND id = $3
`, accountID, namespaceID, current.id); err != nil {
				return false, fmt.Errorf("v2 postgres: archive limit %q: %w", spec.Key, err)
			}
			if _, err := db.Exec(ctx, `
INSERT INTO kave_v2.limits (
    id, account_id, namespace_id, external_key, generation, source, metric,
    tenant_ref, actor_ref, billing_ref, agent_id, model, feature,
    hard_cap, soft_cap, window_kind, enabled
) VALUES (
    $1, $2, $3, $4, $5, 'operator', $6,
    NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''),
    $13, $14, $15, $16
)
`, newID, accountID, namespaceID, spec.Key, next.generation, spec.Metric,
				spec.Selector.Tenant, spec.Selector.Actor, spec.Selector.BillTo, agentID,
				spec.Selector.Model, spec.Selector.Feature, spec.HardCap, spec.SoftCap, window, spec.Enabled); err != nil {
				return false, fmt.Errorf("v2 postgres: create limit generation %q: %w", spec.Key, err)
			}
		}
	}
	return changed, nil
}

func changedLimitFields(current limitRow, spec corev2.LimitSpec, agentID, window string) []string {
	fields := make([]string, 0, 11)
	checks := []struct {
		field   string
		changed bool
	}{
		{"metric", current.metric != string(spec.Metric)},
		{"tenant", current.tenant != string(spec.Selector.Tenant)},
		{"actor", current.actor != string(spec.Selector.Actor)},
		{"bill_to", current.billTo != string(spec.Selector.BillTo)},
		{"agent", current.agentID != agentID},
		{"model", current.model != string(spec.Selector.Model)},
		{"feature", current.feature != string(spec.Selector.Feature)},
		{"window", current.window != window},
		{"hard_cap", current.hardCap != spec.HardCap},
		{"soft_cap", !equalInt64(current.softCap, spec.SoftCap)},
		{"enabled", current.enabled != spec.Enabled},
	}
	for _, check := range checks {
		if check.changed {
			fields = append(fields, check.field)
		}
	}
	slices.Sort(fields)
	return fields
}

func pruneApplyState(ctx context.Context, db DBTX, accountID, namespaceID corev2.Ref, manifest corev2.Manifest, state *applyState, changes *[]corev2.Change, dryRun bool) (bool, error) {
	desiredLimits := make(map[string]struct{}, len(manifest.Limits))
	for _, spec := range manifest.Limits {
		desiredLimits[string(spec.Key)] = struct{}{}
	}
	desiredAgents := make(map[string]struct{}, len(manifest.Agents))
	for _, spec := range manifest.Agents {
		desiredAgents[string(spec.Name)] = struct{}{}
	}
	desiredRoutes := make(map[string]struct{}, len(manifest.Routes))
	for _, spec := range manifest.Routes {
		desiredRoutes[string(spec.Name)] = struct{}{}
	}

	changed := false
	for name, row := range state.limits {
		if row.source != "operator" || row.superseded {
			continue
		}
		if _, exists := desiredLimits[name]; exists {
			continue
		}
		changed = true
		*changes = append(*changes, corev2.Change{Kind: corev2.ChangeDelete, ResourceKind: "limit", Name: corev2.Ref(name), Fields: []string{"enabled"}})
		if !dryRun {
			if _, err := db.Exec(ctx, `UPDATE kave_v2.limits SET enabled = FALSE, superseded_at = transaction_timestamp() WHERE account_id = $1 AND namespace_id = $2 AND id = $3`, accountID, namespaceID, row.id); err != nil {
				return false, fmt.Errorf("v2 postgres: prune limit %q: %w", name, err)
			}
		}
	}
	for name, row := range state.agents {
		if row.status == "archived" {
			continue
		}
		if _, exists := desiredAgents[name]; exists {
			continue
		}
		changed = true
		*changes = append(*changes, corev2.Change{Kind: corev2.ChangeDelete, ResourceKind: "agent", Name: corev2.Ref(name), Fields: []string{"status"}})
		if !dryRun {
			if _, err := db.Exec(ctx, `UPDATE kave_v2.agents SET status = 'archived', revision = revision + 1 WHERE account_id = $1 AND namespace_id = $2 AND id = $3`, accountID, namespaceID, row.id); err != nil {
				return false, fmt.Errorf("v2 postgres: prune agent %q: %w", name, err)
			}
		}
	}
	for name, row := range state.routes {
		if row.status == "archived" {
			continue
		}
		if _, exists := desiredRoutes[name]; exists {
			continue
		}
		changed = true
		*changes = append(*changes, corev2.Change{Kind: corev2.ChangeDelete, ResourceKind: "route", Name: corev2.Ref(name), Fields: []string{"status"}})
		if !dryRun {
			if _, err := db.Exec(ctx, `UPDATE kave_v2.provider_routes SET status = 'archived', revision = revision + 1 WHERE account_id = $1 AND namespace_id = $2 AND id = $3`, accountID, namespaceID, row.id); err != nil {
				return false, fmt.Errorf("v2 postgres: prune route %q: %w", name, err)
			}
		}
	}
	return changed, nil
}

func limitWindowKind(window corev2.Window) string {
	switch window {
	case corev2.WindowDay:
		return "calendar_day"
	case corev2.WindowMonth:
		return "calendar_month"
	default:
		return "lifetime"
	}
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func equalInt64(a, b *int64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func sortChanges(changes []corev2.Change) {
	resourceOrder := func(kind string) int {
		switch kind {
		case "namespace":
			return 0
		case "route":
			return 1
		case "agent":
			return 2
		case "limit":
			return 3
		default:
			return 4
		}
	}
	slices.SortStableFunc(changes, func(a, b corev2.Change) int {
		if order := resourceOrder(a.ResourceKind) - resourceOrder(b.ResourceKind); order != 0 {
			return order
		}
		return strings.Compare(string(a.Name), string(b.Name))
	})
}

var _ corev2.ApplyStore = (*ApplyStore)(nil)
