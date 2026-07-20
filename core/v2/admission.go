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

var ErrUnauthorized = errors.New("kave v2: unauthorized")

type Operation string

const (
	OperationConsume      Operation = "consume"
	OperationInvoke       Operation = "invoke"
	OperationConfigApply  Operation = "config.apply"
	OperationSecretsWrite Operation = "secrets.write"
	OperationKeysManage   Operation = "keys.manage"
	OperationLimitsSync   Operation = "limits.sync"
	OperationUsageRead    Operation = "usage.read"
	OperationAuditRead    Operation = "audit.read"

	// OperationApply is retained as a source-compatible name for manifest
	// reconciliation. Its persisted capability is the narrow "config.apply",
	// not the namespace-root configuration authority.
	OperationApply = OperationConfigApply
)

// Caller is the already-authenticated service identity supplied to the kernel.
// Provider credentials and raw service keys never enter this value.
type Caller struct {
	AccountID       Ref
	NamespaceID     Ref
	ServiceKeyID    Ref
	Operations      []Operation
	AllowedAgents   []Ref // optional name allowlist for embedded stores
	AllowedAgentIDs []Ref // production allowlist, enforced after name resolution
	CanAssertScope  bool
	// Bootstrap identifies the explicitly configured, account-scoped initial
	// control credential. It is never valid for Consume or Invoke and is kept
	// separate from persisted namespace service keys.
	Bootstrap bool
}

func (c Caller) Allows(operation Operation, agent Ref) bool {
	if !slices.Contains(c.Operations, operation) {
		return false
	}
	if c.Bootstrap && operation != OperationConfigApply {
		return false
	}
	if operation != OperationConsume && operation != OperationInvoke {
		return true
	}
	if len(c.AllowedAgents) > 0 {
		return slices.Contains(c.AllowedAgents, agent)
	}
	// ID capabilities are checked by the production store after resolving the
	// asserted agent name inside the caller's RLS scope.
	return len(c.AllowedAgentIDs) > 0
}

type ConsumeRequest struct {
	Caller         Caller `json:"-"`
	Agent          Ref    `json:"agent"`
	Model          Ref    `json:"model,omitempty"`
	Scope          Scope  `json:"scope"`
	Metric         Metric `json:"metric"`
	Units          int64  `json:"units"`
	IdempotencyKey Ref    `json:"idempotency_key"`
}

func (r ConsumeRequest) Validate() error {
	if err := r.Caller.AccountID.Validate("caller.account_id", true); err != nil {
		return err
	}
	if err := r.Caller.NamespaceID.Validate("caller.namespace_id", true); err != nil {
		return err
	}
	if err := r.Caller.ServiceKeyID.Validate("caller.service_key_id", true); err != nil {
		return err
	}
	if err := r.Agent.ValidateName("agent", true); err != nil {
		return err
	}
	if err := r.Model.Validate("model", false); err != nil {
		return err
	}
	if err := r.Scope.ValidateAdmission(); err != nil {
		return err
	}
	if err := r.Metric.Validate(); err != nil {
		return err
	}
	if r.Units <= 0 {
		return invalid("units", "must be greater than zero")
	}
	if err := r.IdempotencyKey.Validate("idempotency_key", true); err != nil {
		return err
	}
	if !r.Caller.Allows(OperationConsume, r.Agent) {
		return ErrUnauthorized
	}
	if !r.Caller.CanAssertScope {
		return fmt.Errorf("%w: service key cannot assert the required admission scope", ErrUnauthorized)
	}
	return nil
}

// Hash is persisted with the idempotency key so reuse with different input is
// rejected rather than silently returning an unrelated decision.
func (r ConsumeRequest) Hash() (string, error) {
	canonical := struct {
		Agent  Ref    `json:"agent"`
		Model  Ref    `json:"model,omitempty"`
		Scope  Scope  `json:"scope"`
		Metric Metric `json:"metric"`
		Units  int64  `json:"units"`
	}{r.Agent, r.Model, r.Scope, r.Metric, r.Units}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("hash consume request: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

type DecisionStatus string

const (
	DecisionAdmitted DecisionStatus = "admitted"
	DecisionRejected DecisionStatus = "rejected"
)

type Violation struct {
	LimitID   string `json:"limit_id"`
	LimitKey  Ref    `json:"limit_key"`
	Metric    Metric `json:"metric"`
	Used      int64  `json:"used"`
	Requested int64  `json:"requested"`
	HardCap   int64  `json:"hard_cap"`
	ResetAt   int64  `json:"reset_at,omitempty"`
}

type Warning struct {
	LimitID  string `json:"limit_id"`
	LimitKey Ref    `json:"limit_key"`
	Used     int64  `json:"used"`
	SoftCap  int64  `json:"soft_cap"`
	ResetAt  int64  `json:"reset_at,omitempty"`
}

type Decision struct {
	InvocationID string         `json:"invocation_id"`
	Status       DecisionStatus `json:"status"`
	Replayed     bool           `json:"replayed,omitempty"`
	Warnings     []Warning      `json:"warnings,omitempty"`
	Violations   []Violation    `json:"violations,omitempty"`
}

type LimitExceededError struct {
	Decision Decision
}

func (e *LimitExceededError) Error() string {
	if e == nil || len(e.Decision.Violations) == 0 {
		return ErrLimitExceeded.Error()
	}
	v := e.Decision.Violations[0]
	return fmt.Sprintf("%s: %s (%d used + %d requested > %d)", ErrLimitExceeded, v.LimitKey, v.Used, v.Requested, v.HardCap)
}

func (e *LimitExceededError) Unwrap() error { return ErrLimitExceeded }

type IdempotencyConflictError struct {
	Key Ref
}

func (e *IdempotencyConflictError) Error() string {
	return fmt.Sprintf("%s: key %q was already used with different input", ErrIdempotencyConflict, e.Key)
}

func (e *IdempotencyConflictError) Unwrap() error { return ErrIdempotencyConflict }

// AdmissionStore owns the atomicity boundary. Production implementations must
// combine idempotency, matching-limit checks, counter updates, invocation, and
// usage ledger writes in one transaction.
type AdmissionStore interface {
	Consume(context.Context, ConsumeRequest) (Decision, error)
}

type AdmissionService struct {
	store AdmissionStore
}

func NewAdmissionService(store AdmissionStore) *AdmissionService {
	return &AdmissionService{store: store}
}

func (s *AdmissionService) Consume(ctx context.Context, req ConsumeRequest) (Decision, error) {
	if err := req.Validate(); err != nil {
		return Decision{}, err
	}
	if s == nil || s.store == nil {
		return Decision{}, errors.New("kave v2: admission store unavailable")
	}
	return s.store.Consume(ctx, req)
}

type BoundLimit struct {
	ID          string
	AccountID   Ref
	NamespaceID Ref
	Spec        LimitSpec
}

func (l BoundLimit) Matches(req ConsumeRequest) bool {
	if !l.Spec.Enabled || l.AccountID != req.Caller.AccountID || l.NamespaceID != req.Caller.NamespaceID || l.Spec.Metric != req.Metric {
		return false
	}
	s := l.Spec.Selector
	return matches(s.Tenant, req.Scope.Tenant) &&
		matches(s.Actor, req.Scope.Actor) &&
		matches(s.BillTo, req.Scope.BillTo) &&
		matches(s.Agent, req.Agent) &&
		matches(s.Model, req.Model) &&
		matches(s.Feature, req.Scope.Feature)
}

func matches(selector, actual Ref) bool { return selector == "" || selector == actual }
