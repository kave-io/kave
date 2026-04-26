package policy_test

import (
	"context"
	"testing"

	"github.com/kave-io/kave/cli/internal/commands/policy"
	"github.com/kave-io/kave/cli/internal/testutil"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type stubControlSvc struct {
	controlv1.UnimplementedControlPlaneServiceServer
	policies []*controlv1.PolicyRecord
}

func (s *stubControlSvc) GetPolicy(_ context.Context, req *controlv1.GetPolicyRequest) (*controlv1.PolicyRecord, error) {
	for _, p := range s.policies {
		if p.Id == req.Id {
			return p, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "policy %q not found", req.Id)
}

func (s *stubControlSvc) ListPolicies(_ context.Context, _ *controlv1.ListPoliciesRequest) (*controlv1.ListPoliciesResponse, error) {
	return &controlv1.ListPoliciesResponse{Policies: s.policies}, nil
}

func TestPolicyGet_gRPC(t *testing.T) {
	stub := &stubControlSvc{
		policies: []*controlv1.PolicyRecord{{Id: "pol-1", Name: "default"}},
	}
	h := testutil.NewGRPCHarness(t, nil, stub)
	ctx := h.Context()

	out, err := policy.RunGet(ctx, policy.GetInput{ID: "pol-1"})
	if err != nil {
		t.Fatalf("RunGet: %v", err)
	}
	if out.Data == nil || out.Data.Id != "pol-1" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestPolicyGet_NotFound(t *testing.T) {
	stub := &stubControlSvc{}
	h := testutil.NewGRPCHarness(t, nil, stub)
	ctx := h.Context()

	_, err := policy.RunGet(ctx, policy.GetInput{ID: "missing"})
	if err == nil {
		t.Fatal("expected error for missing policy")
	}
}

func TestPolicyList_gRPC(t *testing.T) {
	stub := &stubControlSvc{
		policies: []*controlv1.PolicyRecord{
			{Id: "pol-1", Name: "default"},
			{Id: "pol-2", Name: "strict"},
		},
	}
	h := testutil.NewGRPCHarness(t, nil, stub)
	ctx := h.Context()

	out, err := policy.RunList(ctx, policy.ListInput{})
	if err != nil {
		t.Fatalf("RunList: %v", err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(out.Items))
	}
}
