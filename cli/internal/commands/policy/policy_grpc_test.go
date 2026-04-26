package policy_test

import (
	"context"
	"testing"

	"github.com/kave-io/kave/cli/internal/commands/policy"
	"github.com/kave-io/kave/cli/internal/testutil"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
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

func (s *stubControlSvc) CreatePolicy(_ context.Context, req *controlv1.CreatePolicyRequest) (*controlv1.PolicyRecord, error) {
	rec := &controlv1.PolicyRecord{Id: "pol-new", Name: req.Name, EnvId: req.EnvId, Mode: req.Mode}
	s.policies = append(s.policies, rec)
	return rec, nil
}

func (s *stubControlSvc) UpdatePolicy(_ context.Context, req *controlv1.UpdatePolicyRequest) (*controlv1.PolicyRecord, error) {
	for _, p := range s.policies {
		if p.Id == req.Id {
			if req.Update != nil && req.Update.Description != nil {
				p.Description = *req.Update.Description
			}
			return p, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "policy %q not found", req.Id)
}

func (s *stubControlSvc) DeletePolicy(_ context.Context, req *controlv1.DeletePolicyRequest) (*emptypb.Empty, error) {
	for i, p := range s.policies {
		if p.Id == req.Id {
			s.policies = append(s.policies[:i], s.policies[i+1:]...)
			return &emptypb.Empty{}, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "policy %q not found", req.Id)
}

func (s *stubControlSvc) ExportPolicy(_ context.Context, req *controlv1.ExportPolicyRequest) (*controlv1.PolicyYAML, error) {
	for _, p := range s.policies {
		if p.Id == req.Id {
			return &controlv1.PolicyYAML{Yaml: "name: " + p.Name + "\n"}, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "policy %q not found", req.Id)
}

func (s *stubControlSvc) ValidatePolicy(_ context.Context, req *controlv1.ValidatePolicyRequest) (*controlv1.ValidatePolicyResponse, error) {
	if req.Yaml == "" {
		return &controlv1.ValidatePolicyResponse{Ok: false, Issues: []string{"empty yaml"}}, nil
	}
	return &controlv1.ValidatePolicyResponse{Ok: true}, nil
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

func TestPolicyCreate_gRPC(t *testing.T) {
	stub := &stubControlSvc{}
	h := testutil.NewGRPCHarness(t, nil, stub)
	out, err := policy.RunCreate(h.Context(), policy.CreateInput{EnvID: "env-1", Name: "p", Mode: "enforce"})
	if err != nil {
		t.Fatalf("RunCreate: %v", err)
	}
	if out.Data == nil || out.Data.Mode != controlv1.PolicyMode_POLICY_MODE_ENFORCE {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestPolicyUpdate_Description(t *testing.T) {
	stub := &stubControlSvc{policies: []*controlv1.PolicyRecord{{Id: "p1", Name: "x"}}}
	h := testutil.NewGRPCHarness(t, nil, stub)
	desc := "new"
	out, err := policy.RunUpdate(h.Context(), policy.UpdateInput{ID: "p1", Description: &desc})
	if err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	if out.Data.Description != "new" {
		t.Fatalf("description not applied: %+v", out)
	}
}

func TestPolicyDelete_gRPC(t *testing.T) {
	stub := &stubControlSvc{policies: []*controlv1.PolicyRecord{{Id: "p1"}}}
	h := testutil.NewGRPCHarness(t, nil, stub)
	out, err := policy.RunDelete(h.Context(), policy.DeleteInput{ID: "p1"})
	if err != nil {
		t.Fatalf("RunDelete: %v", err)
	}
	if !out.Deleted || len(stub.policies) != 0 {
		t.Fatalf("not deleted: %+v left=%d", out, len(stub.policies))
	}
}

func TestPolicyExport_gRPC(t *testing.T) {
	stub := &stubControlSvc{policies: []*controlv1.PolicyRecord{{Id: "p1", Name: "alpha"}}}
	h := testutil.NewGRPCHarness(t, nil, stub)
	out, err := policy.RunExport(h.Context(), policy.ExportInput{ID: "p1"})
	if err != nil {
		t.Fatalf("RunExport: %v", err)
	}
	if out.YAML == "" {
		t.Fatal("empty yaml")
	}
}

func TestPolicyValidate_OK(t *testing.T) {
	stub := &stubControlSvc{}
	h := testutil.NewGRPCHarness(t, nil, stub)
	out, err := policy.RunValidate(h.Context(), policy.ValidateInput{YAML: "name: x\n"})
	if err != nil {
		t.Fatalf("RunValidate: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected ok, got %+v", out)
	}
}

func TestPolicyValidate_RequiresYAML(t *testing.T) {
	stub := &stubControlSvc{}
	h := testutil.NewGRPCHarness(t, nil, stub)
	if _, err := policy.RunValidate(h.Context(), policy.ValidateInput{}); err == nil {
		t.Fatal("expected error")
	}
}
