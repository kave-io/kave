package v2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

var ErrRevisionConflict = errors.New("kave v2: revision conflict")

const (
	maxManifestRoutes         = 128
	maxManifestAgents         = 128
	maxManifestLimits         = 512
	maxRouteModelsPerManifest = 256
)

// ChangeKind describes the observable effect of one declarative Apply. Delete
// means that a resource was removed from the desired active state; stores may
// retain and disable it when physical deletion would break ledger history.
type ChangeKind string

const (
	ChangeCreate    ChangeKind = "create"
	ChangeUpdate    ChangeKind = "update"
	ChangeDelete    ChangeKind = "delete"
	ChangeUnchanged ChangeKind = "unchanged"
)

type Change struct {
	Kind         ChangeKind `json:"kind"`
	ResourceKind string     `json:"resource_kind"`
	Name         Ref        `json:"name"`
	Fields       []string   `json:"fields,omitempty"`
}

type ApplyRequest struct {
	Caller           Caller   `json:"-"`
	Manifest         Manifest `json:"manifest"`
	DryRun           bool     `json:"dry_run,omitempty"`
	Prune            bool     `json:"prune,omitempty"`
	ExpectedRevision int64    `json:"expected_revision,omitempty"`
	IdempotencyKey   Ref      `json:"idempotency_key"`
}

func (r ApplyRequest) Validate() error {
	if len(r.Manifest.Routes) > maxManifestRoutes {
		return invalid("manifest.routes", fmt.Sprintf("must contain at most %d entries", maxManifestRoutes))
	}
	if len(r.Manifest.Agents) > maxManifestAgents {
		return invalid("manifest.agents", fmt.Sprintf("must contain at most %d entries", maxManifestAgents))
	}
	if len(r.Manifest.Limits) > maxManifestLimits {
		return invalid("manifest.limits", fmt.Sprintf("must contain at most %d entries", maxManifestLimits))
	}
	for i, route := range r.Manifest.Routes {
		if len(route.AllowedModels) > maxRouteModelsPerManifest {
			return invalid(fmt.Sprintf("manifest.routes[%d].allowed_models", i), fmt.Sprintf("must contain at most %d entries", maxRouteModelsPerManifest))
		}
		if len(route.Pricing) > maxRouteModelsPerManifest {
			return invalid(fmt.Sprintf("manifest.routes[%d].pricing", i), fmt.Sprintf("must contain at most %d entries", maxRouteModelsPerManifest))
		}
	}
	if err := r.Manifest.Validate(); err != nil {
		return err
	}
	if err := r.Caller.AccountID.Validate("caller.account_id", true); err != nil {
		return err
	}
	if r.Caller.Bootstrap {
		if r.Caller.NamespaceID != "" {
			return fmt.Errorf("%w: bootstrap caller must remain account-scoped", ErrUnauthorized)
		}
		if err := r.Caller.ServiceKeyID.Validate("caller.service_key_id", false); err != nil {
			return err
		}
	} else {
		if err := r.Caller.NamespaceID.Validate("caller.namespace_id", true); err != nil {
			return err
		}
		if err := r.Caller.ServiceKeyID.Validate("caller.service_key_id", true); err != nil {
			return err
		}
	}
	if !r.Caller.Allows(OperationApply, "") {
		return ErrUnauthorized
	}
	if r.Caller.AccountID != r.Manifest.Namespace.Account {
		return fmt.Errorf("%w: caller account does not match manifest namespace", ErrUnauthorized)
	}
	if r.ExpectedRevision < 0 {
		return invalid("expected_revision", "must not be negative")
	}
	return r.IdempotencyKey.Validate("idempotency_key", true)
}

// CanonicalManifest returns a deep, deterministically ordered copy suitable
// for hashing, diffing, and persistence. Resource ordering and duplicate model
// names do not change the meaning of a manifest.
func (r ApplyRequest) CanonicalManifest() Manifest {
	m := r.Manifest
	m.Namespace.ID = ""
	m.Routes = slices.Clone(m.Routes)
	m.Agents = slices.Clone(m.Agents)
	m.Limits = slices.Clone(m.Limits)

	for i := range m.Routes {
		models := slices.Clone(m.Routes[i].AllowedModels)
		slices.Sort(models)
		models = slices.Compact(models)
		m.Routes[i].AllowedModels = models
		m.Routes[i].Pricing = slices.Clone(m.Routes[i].Pricing)
		slices.SortFunc(m.Routes[i].Pricing, func(a, b ModelPrice) int {
			return stringCompare(a.Model, b.Model)
		})
	}
	slices.SortFunc(m.Routes, func(a, b RouteSpec) int { return stringCompare(a.Name, b.Name) })
	slices.SortFunc(m.Agents, func(a, b AgentSpec) int { return stringCompare(a.Name, b.Name) })
	slices.SortFunc(m.Limits, func(a, b LimitSpec) int { return stringCompare(a.Key, b.Key) })
	return m
}

// Hash binds an idempotency key to all state-changing request semantics. A
// dry-run is intentionally excluded because it never persists an idempotency
// record and can safely precede the real Apply with the same key.
func (r ApplyRequest) Hash() (string, error) {
	canonical := struct {
		Manifest         Manifest `json:"manifest"`
		Prune            bool     `json:"prune"`
		ExpectedRevision int64    `json:"expected_revision"`
	}{r.CanonicalManifest(), r.Prune, r.ExpectedRevision}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("hash apply request: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

type ApplyResult struct {
	NamespaceID Ref      `json:"namespace_id"`
	Revision    int64    `json:"revision"`
	Applied     bool     `json:"applied"`
	Changes     []Change `json:"changes,omitempty"`
}

type RevisionConflictError struct {
	Expected int64
	Actual   int64
}

func (e *RevisionConflictError) Error() string {
	if e == nil {
		return ErrRevisionConflict.Error()
	}
	return fmt.Sprintf("%s: expected %d, current revision is %d", ErrRevisionConflict, e.Expected, e.Actual)
}

func (e *RevisionConflictError) Unwrap() error { return ErrRevisionConflict }

type ApplyStore interface {
	Apply(context.Context, ApplyRequest) (ApplyResult, error)
}

type ApplyService struct {
	store ApplyStore
}

func NewApplyService(store ApplyStore) *ApplyService { return &ApplyService{store: store} }

func (s *ApplyService) Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error) {
	if err := req.Validate(); err != nil {
		return ApplyResult{}, err
	}
	if s == nil || s.store == nil {
		return ApplyResult{}, errors.New("kave v2: apply store unavailable")
	}
	return s.store.Apply(ctx, req)
}

func stringCompare(a, b Ref) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
