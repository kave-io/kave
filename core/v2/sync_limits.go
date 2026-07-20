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

const maxSyncedLimits = 512

var ErrLimitOwnershipConflict = errors.New("kave v2: limit ownership conflict")

// SyncLimitsRequest replaces the complete desired limit set owned by one
// external source. Revisions are monotonic per namespace and owner; omission
// disables a previously current limit owned by that same source.
type SyncLimitsRequest struct {
	Caller         Caller
	NamespaceID    Ref
	Owner          Ref
	Revision       int64
	Limits         []LimitSpec
	IdempotencyKey Ref
}

func (r SyncLimitsRequest) ValidateRequest() error {
	// Subscription materialization is a namespace operation, not a bootstrap
	// operation. This prevents the account-scoped bootstrap key from becoming a
	// standing cross-namespace limit writer.
	if r.Caller.Bootstrap {
		return fmt.Errorf("%w: bootstrap credential cannot synchronize limits", ErrUnauthorized)
	}
	if err := r.Caller.AuthorizeControl(r.NamespaceID, OperationLimitsSync); err != nil {
		return err
	}
	if err := r.Owner.Validate("limit_owner", true); err != nil {
		return err
	}
	if r.Owner == "operator" {
		return invalid("limit_owner", "operator is reserved for declarative Apply")
	}
	if r.Revision <= 0 {
		return invalid("revision", "must be greater than zero")
	}
	if err := r.IdempotencyKey.Validate("idempotency_key", true); err != nil {
		return err
	}
	if len(r.Limits) > maxSyncedLimits {
		return invalid("limits", fmt.Sprintf("must contain at most %d entries", maxSyncedLimits))
	}
	keys := make(map[Ref]struct{}, len(r.Limits))
	for i, limit := range r.Limits {
		if err := limit.Validate(); err != nil {
			return fmt.Errorf("limit[%d]: %w", i, err)
		}
		if _, exists := keys[limit.Key]; exists {
			return invalid("limits", fmt.Sprintf("contains duplicate key %q", limit.Key))
		}
		keys[limit.Key] = struct{}{}
	}
	return nil
}

// CanonicalLimits returns an independently owned, stable ordering used for
// request hashing. Callers may submit limits in any order without changing the
// meaning of an idempotent synchronization.
func (r SyncLimitsRequest) CanonicalLimits() []LimitSpec {
	limits := slices.Clone(r.Limits)
	for i := range limits {
		limits[i].SoftCap = cloneOptionalInt64(limits[i].SoftCap)
	}
	slices.SortFunc(limits, func(a, b LimitSpec) int { return stringCompare(a.Key, b.Key) })
	return limits
}

func (r SyncLimitsRequest) Hash() (string, error) {
	canonical := struct {
		NamespaceID Ref         `json:"namespace_id"`
		Owner       Ref         `json:"owner"`
		Revision    int64       `json:"revision"`
		Limits      []LimitSpec `json:"limits"`
	}{r.NamespaceID, r.Owner, r.Revision, r.CanonicalLimits()}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("hash limit synchronization: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

type SyncLimitsResult struct {
	Revision int64 `json:"revision"`
	Created  int32 `json:"created"`
	Updated  int32 `json:"updated"`
	Disabled int32 `json:"disabled"`
	Replayed bool  `json:"replayed,omitempty"`
}

type LimitOwnershipConflictError struct {
	Key Ref
}

func (e *LimitOwnershipConflictError) Error() string {
	if e == nil {
		return ErrLimitOwnershipConflict.Error()
	}
	return fmt.Sprintf("%s: key %q belongs to another source", ErrLimitOwnershipConflict, e.Key)
}

func (e *LimitOwnershipConflictError) Unwrap() error { return ErrLimitOwnershipConflict }

// SourceRevisionConflictError means an owner attempted to submit a revision
// older than its current materialization, or reused the current revision for a
// different desired set. It unwraps to ErrRevisionConflict so every transport
// exposes the same optimistic-concurrency error class.
type SourceRevisionConflictError struct {
	Owner     Ref
	Requested int64
	Current   int64
}

func (e *SourceRevisionConflictError) Error() string {
	if e == nil {
		return ErrRevisionConflict.Error()
	}
	return fmt.Sprintf("%s: limit owner %q submitted revision %d; current revision is %d",
		ErrRevisionConflict, e.Owner, e.Requested, e.Current)
}

func (e *SourceRevisionConflictError) Unwrap() error { return ErrRevisionConflict }

type LimitSyncStore interface {
	SyncLimits(context.Context, SyncLimitsRequest) (SyncLimitsResult, error)
}

type LimitSyncService struct{ store LimitSyncStore }

func NewLimitSyncService(store LimitSyncStore) *LimitSyncService {
	return &LimitSyncService{store: store}
}

func (s *LimitSyncService) Sync(ctx context.Context, req SyncLimitsRequest) (SyncLimitsResult, error) {
	if err := req.ValidateRequest(); err != nil {
		return SyncLimitsResult{}, err
	}
	if s == nil || s.store == nil {
		return SyncLimitsResult{}, errors.New("kave v2: limit synchronization store unavailable")
	}
	return s.store.SyncLimits(ctx, req)
}

func cloneOptionalInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
