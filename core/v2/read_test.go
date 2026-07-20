package v2

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func readAdmin() Caller {
	return Caller{
		AccountID: "account/acme", NamespaceID: "nsp_prod", ServiceKeyID: "key_admin",
		Operations: []Operation{OperationConfigApply, OperationUsageRead, OperationAuditRead}, CanAssertScope: true,
	}
}

func readScope() Scope { return Scope{Tenant: "clinic/opaque", BillTo: "clinic/opaque"} }

func readRange() TimeRange {
	return TimeRange{From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}
}

func TestReadQueriesRequireAdminAndLeakBoundaries(t *testing.T) {
	t.Parallel()
	request := QueryUsageRequest{Caller: readAdmin(), Scope: readScope(), Range: readRange()}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}

	request.Scope.BillTo = ""
	if err := request.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("missing bill_to error = %v", err)
	}
	request.Scope = readScope()
	request.Caller.Operations = []Operation{OperationConsume}
	if err := request.Validate(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("non-admin error = %v", err)
	}
}

func TestLimitStatusMayUseScopedWorkloadKey(t *testing.T) {
	t.Parallel()
	caller := Caller{
		AccountID: "account/acme", NamespaceID: "nsp_prod", ServiceKeyID: "key_worker",
		Operations: []Operation{OperationConsume}, AllowedAgents: []Ref{"assistant"}, CanAssertScope: true,
	}
	req := GetLimitStatusRequest{Caller: caller, Scope: readScope(), Agent: "assistant", Metric: "ai_actions"}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	req.Agent = "other"
	if err := req.Validate(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("disallowed agent error = %v", err)
	}
}

func TestReadPaginationAndRangesAreBounded(t *testing.T) {
	t.Parallel()
	if got := (Page{}).EffectiveSize(); got != DefaultReadPageSize {
		t.Fatalf("default page size = %d", got)
	}
	if err := (Page{Size: MaxReadPageSize + 1}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("oversized page error = %v", err)
	}
	if err := (Page{Token: strings.Repeat("x", MaxReadPageToken+1)}).Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("oversized token error = %v", err)
	}
	if err := (TimeRange{From: time.Now().Add(-MaxReadRange - time.Hour), To: time.Now()}).validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("oversized range error = %v", err)
	}
}

func TestListTenantsRequiresUsageReadAndBoundedRange(t *testing.T) {
	t.Parallel()
	req := ListTenantsRequest{Caller: readAdmin(), Range: readRange(), Page: Page{Size: 25}}
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}

	req.Caller.Operations = []Operation{OperationAuditRead}
	if err := req.Validate(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("audit-only caller error = %v", err)
	}

	req.Caller = readAdmin()
	req.Range.From = req.Range.To.Add(-MaxReadRange - time.Millisecond)
	if err := req.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("oversized tenant range error = %v", err)
	}

	req.Range = readRange()
	req.Page.Size = MaxReadPageSize + 1
	if err := req.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("oversized tenant page error = %v", err)
	}
}

func TestGetStateIsNamespaceBound(t *testing.T) {
	t.Parallel()
	if err := (GetStateRequest{Caller: readAdmin(), NamespaceID: "nsp_prod"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (GetStateRequest{Caller: readAdmin(), NamespaceID: "nsp_other"}).Validate(); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross namespace error = %v", err)
	}
}
