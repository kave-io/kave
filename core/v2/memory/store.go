// Package memory provides a deterministic, concurrency-safe V2 admission store
// for unit tests and local embeddings. Production admission uses Postgres.
package memory

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/kave-io/kave/core/pkg/ids"
	v2 "github.com/kave-io/kave/core/v2"
)

type Store struct {
	mu          sync.Mutex
	now         func() time.Time
	limits      []v2.BoundLimit
	counters    map[counterKey]int64
	idempotency map[idempotencyKey]idempotencyRecord
	invocations []Invocation
	usage       []UsageEntry
}

type counterKey struct {
	limitID     string
	windowStart int64
}

type idempotencyKey struct {
	accountID   string
	namespaceID string
	operation   v2.Operation
	key         v2.Ref
}

type idempotencyRecord struct {
	hash     string
	decision v2.Decision
}

type Invocation struct {
	ID             string
	AccountID      v2.Ref
	NamespaceID    v2.Ref
	ServiceKeyID   v2.Ref
	Agent          v2.Ref
	Scope          v2.Scope
	Metric         v2.Metric
	Units          int64
	IdempotencyKey v2.Ref
	RequestHash    string
	Status         v2.DecisionStatus
	CreatedAt      int64
}

type UsageEntry struct {
	ID              string
	InvocationID    string
	AccountID       v2.Ref
	NamespaceID     v2.Ref
	Metric          v2.Metric
	Units           int64
	AppliedLimitIDs []string
	CreatedAt       int64
}

type Snapshot struct {
	Counters    map[string]int64
	Invocations []Invocation
	Usage       []UsageEntry
}

func New(limits ...v2.BoundLimit) *Store {
	return NewWithClock(time.Now, limits...)
}

func NewWithClock(now func() time.Time, limits ...v2.BoundLimit) *Store {
	if now == nil {
		now = time.Now
	}
	copyLimits := append([]v2.BoundLimit(nil), limits...)
	slices.SortFunc(copyLimits, func(a, b v2.BoundLimit) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return &Store{
		now:         now,
		limits:      copyLimits,
		counters:    make(map[counterKey]int64),
		idempotency: make(map[idempotencyKey]idempotencyRecord),
	}
}

func (s *Store) Consume(_ context.Context, req v2.ConsumeRequest) (v2.Decision, error) {
	hash, err := req.Hash()
	if err != nil {
		return v2.Decision{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idemKey := idempotencyKey{
		accountID:   string(req.Caller.AccountID),
		namespaceID: string(req.Caller.NamespaceID),
		operation:   v2.OperationConsume,
		key:         req.IdempotencyKey,
	}
	if prior, exists := s.idempotency[idemKey]; exists {
		if prior.hash != hash {
			return v2.Decision{}, &v2.IdempotencyConflictError{Key: req.IdempotencyKey}
		}
		decision := cloneDecision(prior.decision)
		decision.Replayed = true
		if decision.Status == v2.DecisionRejected {
			return decision, &v2.LimitExceededError{Decision: decision}
		}
		return decision, nil
	}

	now := s.now().UTC()
	type applicableLimit struct {
		limit v2.BoundLimit
		key   counterKey
		used  int64
		end   int64
	}
	applicable := make([]applicableLimit, 0, len(s.limits))
	for _, limit := range s.limits {
		if !limit.Matches(req) {
			continue
		}
		start, end := window(now, limit.Spec.Window)
		key := counterKey{limitID: limit.ID, windowStart: start}
		applicable = append(applicable, applicableLimit{
			limit: limit,
			key:   key,
			used:  s.counters[key],
			end:   end,
		})
	}

	decision := v2.Decision{InvocationID: ids.New("ivk"), Status: v2.DecisionAdmitted}
	for _, item := range applicable {
		overflow := req.Units > math.MaxInt64-item.used
		prospective := item.used + req.Units
		if overflow || prospective > item.limit.Spec.HardCap {
			decision.Violations = append(decision.Violations, v2.Violation{
				LimitID:   item.limit.ID,
				LimitKey:  item.limit.Spec.Key,
				Metric:    req.Metric,
				Used:      item.used,
				Requested: req.Units,
				HardCap:   item.limit.Spec.HardCap,
				ResetAt:   item.end,
			})
			continue
		}
		if item.limit.Spec.SoftCap != nil && prospective > *item.limit.Spec.SoftCap {
			decision.Warnings = append(decision.Warnings, v2.Warning{
				LimitID:  item.limit.ID,
				LimitKey: item.limit.Spec.Key,
				Used:     prospective,
				SoftCap:  *item.limit.Spec.SoftCap,
				ResetAt:  item.end,
			})
		}
	}

	if len(decision.Violations) > 0 {
		decision.Status = v2.DecisionRejected
	}

	s.invocations = append(s.invocations, Invocation{
		ID:             decision.InvocationID,
		AccountID:      req.Caller.AccountID,
		NamespaceID:    req.Caller.NamespaceID,
		ServiceKeyID:   req.Caller.ServiceKeyID,
		Agent:          req.Agent,
		Scope:          req.Scope,
		Metric:         req.Metric,
		Units:          req.Units,
		IdempotencyKey: req.IdempotencyKey,
		RequestHash:    hash,
		Status:         decision.Status,
		CreatedAt:      now.UnixMilli(),
	})
	s.idempotency[idemKey] = idempotencyRecord{hash: hash, decision: cloneDecision(decision)}

	if decision.Status == v2.DecisionRejected {
		return decision, &v2.LimitExceededError{Decision: cloneDecision(decision)}
	}

	appliedIDs := make([]string, 0, len(applicable))
	for _, item := range applicable {
		s.counters[item.key] = item.used + req.Units
		appliedIDs = append(appliedIDs, item.limit.ID)
	}
	s.usage = append(s.usage, UsageEntry{
		ID:              ids.New("use"),
		InvocationID:    decision.InvocationID,
		AccountID:       req.Caller.AccountID,
		NamespaceID:     req.Caller.NamespaceID,
		Metric:          req.Metric,
		Units:           req.Units,
		AppliedLimitIDs: appliedIDs,
		CreatedAt:       now.UnixMilli(),
	})
	return cloneDecision(decision), nil
}

func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	counters := make(map[string]int64, len(s.counters))
	for key, value := range s.counters {
		counters[key.limitID] += value
	}
	invocations := append([]Invocation(nil), s.invocations...)
	usage := make([]UsageEntry, len(s.usage))
	for i, entry := range s.usage {
		usage[i] = entry
		usage[i].AppliedLimitIDs = append([]string(nil), entry.AppliedLimitIDs...)
	}
	return Snapshot{Counters: counters, Invocations: invocations, Usage: usage}
}

func window(now time.Time, period v2.Window) (start, end int64) {
	switch period {
	case v2.WindowDay:
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return from.UnixMilli(), from.AddDate(0, 0, 1).UnixMilli()
	case v2.WindowMonth:
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return from.UnixMilli(), from.AddDate(0, 1, 0).UnixMilli()
	default:
		return 0, 0
	}
}

func cloneDecision(in v2.Decision) v2.Decision {
	in.Warnings = append([]v2.Warning(nil), in.Warnings...)
	in.Violations = append([]v2.Violation(nil), in.Violations...)
	return in
}

var _ v2.AdmissionStore = (*Store)(nil)
