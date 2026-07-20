package memory_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v2 "github.com/kave-io/kave/core/v2"
	"github.com/kave-io/kave/core/v2/memory"
)

func TestConcurrentConsumeCannotExceedHardCap(t *testing.T) {
	t.Parallel()

	store := memory.New(boundLimit("tenant-five", 5, v2.LimitSelector{Tenant: "clinic/a"}))
	service := v2.NewAdmissionService(store)
	var admitted atomic.Int32
	var rejected atomic.Int32
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := request(fmt.Sprintf("attempt/%d", i), "clinic/a", 1)
			_, err := service.Consume(context.Background(), req)
			switch {
			case err == nil:
				admitted.Add(1)
			case errors.Is(err, v2.ErrLimitExceeded):
				rejected.Add(1)
			default:
				t.Errorf("Consume() error = %v", err)
			}
		}()
	}
	wg.Wait()

	if admitted.Load() != 5 || rejected.Load() != 15 {
		t.Fatalf("admitted=%d rejected=%d, want 5/15", admitted.Load(), rejected.Load())
	}
	snapshot := store.Snapshot()
	if snapshot.Counters["lim_tenant-five"] != 5 || len(snapshot.Usage) != 5 || len(snapshot.Invocations) != 20 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestConsumeIdempotencyConsumesExactlyOnce(t *testing.T) {
	t.Parallel()

	store := memory.New(boundLimit("actions", 10, v2.LimitSelector{}))
	service := v2.NewAdmissionService(store)
	req := request("run/one", "clinic/a", 1)
	first, err := service.Consume(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	rotated := req
	rotated.Caller.ServiceKeyID = "key/worker-rotated"
	second, err := service.Consume(context.Background(), rotated)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || first.InvocationID != second.InvocationID {
		t.Fatalf("unexpected replay: first=%+v second=%+v", first, second)
	}

	changed := req
	changed.Units = 2
	if _, err := service.Consume(context.Background(), changed); !errors.Is(err, v2.ErrIdempotencyConflict) {
		t.Fatalf("changed replay error = %v, want idempotency conflict", err)
	}

	snapshot := store.Snapshot()
	if snapshot.Counters["lim_actions"] != 1 || len(snapshot.Usage) != 1 || len(snapshot.Invocations) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestConsumeIdempotencyIsAccountAndNamespaceScoped(t *testing.T) {
	t.Parallel()
	store := memory.New()
	service := v2.NewAdmissionService(store)
	firstRequest := request("run/shared", "clinic/a", 1)
	first, err := service.Consume(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondRequest := firstRequest
	secondRequest.Caller.AccountID = "account/other"
	second, err := service.Consume(context.Background(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if second.Replayed || second.InvocationID == first.InvocationID {
		t.Fatalf("cross-account request replayed: first=%+v second=%+v", first, second)
	}
}

func TestAllApplicableLimitsUpdateOrNoneDo(t *testing.T) {
	t.Parallel()

	store := memory.New(
		boundLimit("global", 100, v2.LimitSelector{}),
		boundLimit("tenant", 1, v2.LimitSelector{Tenant: "clinic/a"}),
	)
	service := v2.NewAdmissionService(store)
	if _, err := service.Consume(context.Background(), request("run/one", "clinic/a", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Consume(context.Background(), request("run/two", "clinic/a", 1)); !errors.Is(err, v2.ErrLimitExceeded) {
		t.Fatalf("second consume error = %v, want limit exceeded", err)
	}

	snapshot := store.Snapshot()
	if snapshot.Counters["lim_global"] != 1 || snapshot.Counters["lim_tenant"] != 1 {
		t.Fatalf("partial counter update: %+v", snapshot.Counters)
	}
}

func TestTenantSelectorDoesNotAffectAnotherTenant(t *testing.T) {
	t.Parallel()

	store := memory.New(boundLimit("clinic-a", 1, v2.LimitSelector{Tenant: "clinic/a"}))
	service := v2.NewAdmissionService(store)
	for i := range 3 {
		if _, err := service.Consume(context.Background(), request(fmt.Sprintf("run/b/%d", i), "clinic/b", 1)); err != nil {
			t.Fatalf("clinic/b consume %d: %v", i, err)
		}
	}
	if got := store.Snapshot().Counters["lim_clinic-a"]; got != 0 {
		t.Fatalf("clinic/a counter = %d after clinic/b traffic", got)
	}
}

func TestMonthlyWindowAndSoftCap(t *testing.T) {
	t.Parallel()

	soft := int64(1)
	limit := boundLimit("monthly", 3, v2.LimitSelector{})
	limit.Spec.Window = v2.WindowMonth
	limit.Spec.SoftCap = &soft
	store := memory.NewWithClock(func() time.Time {
		return time.Date(2026, time.July, 18, 12, 0, 0, 0, time.FixedZone("IRST", 3*60*60+30*60))
	}, limit)
	service := v2.NewAdmissionService(store)
	decision, err := service.Consume(context.Background(), request("run/soft", "clinic/a", 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Warnings) != 1 {
		t.Fatalf("warnings = %+v, want one", decision.Warnings)
	}
	wantReset := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	if decision.Warnings[0].ResetAt != wantReset {
		t.Fatalf("reset_at = %d, want %d", decision.Warnings[0].ResetAt, wantReset)
	}
}

func boundLimit(key string, hardCap int64, selector v2.LimitSelector) v2.BoundLimit {
	return v2.BoundLimit{
		ID: "lim_" + key, AccountID: "account/acme", NamespaceID: "namespace/prod",
		Spec: v2.LimitSpec{Key: v2.Ref(key), Metric: "ai_actions", Selector: selector, Window: v2.WindowAllTime, HardCap: hardCap, Enabled: true},
	}
}

func request(idempotency, tenant string, units int64) v2.ConsumeRequest {
	return v2.ConsumeRequest{
		Caller: v2.Caller{
			AccountID: "account/acme", NamespaceID: "namespace/prod", ServiceKeyID: "key/worker",
			Operations: []v2.Operation{v2.OperationConsume}, AllowedAgents: []v2.Ref{"clinic-assistant"}, CanAssertScope: true,
		},
		Agent: "clinic-assistant", Scope: v2.Scope{Tenant: v2.Ref(tenant), BillTo: v2.Ref(tenant), Feature: "ai_actions"},
		Metric: "ai_actions", Units: units, IdempotencyKey: v2.Ref(idempotency),
	}
}
