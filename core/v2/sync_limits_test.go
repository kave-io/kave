package v2_test

import (
	"context"
	"errors"
	"testing"

	v2 "github.com/kave-io/kave/core/v2"
)

func TestSyncLimitsHashIsOrderIndependentAndBindsRevision(t *testing.T) {
	t.Parallel()
	a := validSyncLimitsRequest()
	b := a
	b.Limits = []v2.LimitSpec{a.Limits[1], a.Limits[0]}

	aHash, err := a.Hash()
	if err != nil {
		t.Fatal(err)
	}
	bHash, err := b.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if aHash != bHash {
		t.Fatalf("equivalent synchronizations hashed differently: %s != %s", aHash, bHash)
	}
	b.Revision++
	bHash, err = b.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if aHash == bHash {
		t.Fatal("source revision was not bound into request hash")
	}
}

func TestSyncLimitsRequiresNamespaceAdminAndSafeOwner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*v2.SyncLimitsRequest)
		want   error
	}{
		{name: "bootstrap", mutate: func(r *v2.SyncLimitsRequest) { r.Caller.Bootstrap = true }, want: v2.ErrUnauthorized},
		{name: "other namespace", mutate: func(r *v2.SyncLimitsRequest) { r.NamespaceID = "nsp_other" }, want: v2.ErrUnauthorized},
		{name: "missing limits capability", mutate: func(r *v2.SyncLimitsRequest) { r.Caller.Operations = nil }, want: v2.ErrUnauthorized},
		{name: "missing owner", mutate: func(r *v2.SyncLimitsRequest) { r.Owner = "" }, want: v2.ErrInvalidArgument},
		{name: "reserved owner", mutate: func(r *v2.SyncLimitsRequest) { r.Owner = "operator" }, want: v2.ErrInvalidArgument},
		{name: "zero revision", mutate: func(r *v2.SyncLimitsRequest) { r.Revision = 0 }, want: v2.ErrInvalidArgument},
		{name: "missing idempotency", mutate: func(r *v2.SyncLimitsRequest) { r.IdempotencyKey = "" }, want: v2.ErrInvalidArgument},
		{name: "duplicate key", mutate: func(r *v2.SyncLimitsRequest) { r.Limits = append(r.Limits, r.Limits[0]) }, want: v2.ErrInvalidArgument},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := validSyncLimitsRequest()
			test.mutate(&req)
			if err := req.ValidateRequest(); !errors.Is(err, test.want) {
				t.Fatalf("ValidateRequest() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestLimitSyncServiceValidatesBeforeStore(t *testing.T) {
	t.Parallel()
	store := &fakeLimitSyncStore{}
	service := v2.NewLimitSyncService(store)
	req := validSyncLimitsRequest()
	req.Revision = -1
	if _, err := service.Sync(context.Background(), req); !errors.Is(err, v2.ErrInvalidArgument) {
		t.Fatalf("Sync() error = %v, want invalid argument", err)
	}
	if store.calls != 0 {
		t.Fatalf("store called %d times for invalid request", store.calls)
	}
}

func TestLimitSyncErrorsExposeStableClasses(t *testing.T) {
	t.Parallel()
	if err := (&v2.SourceRevisionConflictError{Owner: "subscriptions", Requested: 2, Current: 3}); !errors.Is(err, v2.ErrRevisionConflict) {
		t.Fatalf("errors.Is(%v, ErrRevisionConflict) = false", err)
	}
	if err := (&v2.LimitOwnershipConflictError{Key: "clinic/1"}); !errors.Is(err, v2.ErrLimitOwnershipConflict) {
		t.Fatalf("errors.Is(%v, ErrLimitOwnershipConflict) = false", err)
	}
}

type fakeLimitSyncStore struct{ calls int }

func (s *fakeLimitSyncStore) SyncLimits(context.Context, v2.SyncLimitsRequest) (v2.SyncLimitsResult, error) {
	s.calls++
	return v2.SyncLimitsResult{Revision: 1}, nil
}

func validSyncLimitsRequest() v2.SyncLimitsRequest {
	soft := int64(8)
	return v2.SyncLimitsRequest{
		Caller: v2.Caller{
			AccountID: "account/test", NamespaceID: "nsp_test", ServiceKeyID: "key_test",
			Operations: []v2.Operation{v2.OperationLimitsSync},
		},
		NamespaceID: "nsp_test",
		Owner:       "simorq/subscriptions",
		Revision:    7,
		Limits: []v2.LimitSpec{
			{Key: "clinic/a", Metric: "ai_actions", Selector: v2.LimitSelector{Tenant: "clinic/a"}, Window: v2.WindowMonth, HardCap: 10, SoftCap: &soft, Enabled: true},
			{Key: "user/b", Metric: "ai_actions", Selector: v2.LimitSelector{Actor: "user/b"}, Window: v2.WindowDay, HardCap: 3, Enabled: true},
		},
		IdempotencyKey: "outbox/7",
	}
}
