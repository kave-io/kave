package agent_test

import (
	"context"
	"testing"

	"github.com/kave-io/kave/cli/internal/commands/agent"
	"github.com/kave-io/kave/cli/internal/testutil"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type stubControlSvc struct {
	controlv1.UnimplementedControlPlaneServiceServer
	agents      []*controlv1.Agent
	deletedIDs  []string
	restoredIDs []string
}

func (s *stubControlSvc) DeleteAgent(_ context.Context, req *controlv1.DeleteAgentRequest) (*emptypb.Empty, error) {
	for _, a := range s.agents {
		if a.Id == req.Id {
			s.deletedIDs = append(s.deletedIDs, req.Id)
			return &emptypb.Empty{}, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "agent %q not found", req.Id)
}

func (s *stubControlSvc) RestoreAgent(_ context.Context, req *controlv1.RestoreAgentRequest) (*controlv1.Agent, error) {
	s.restoredIDs = append(s.restoredIDs, req.Id)
	return &controlv1.Agent{Id: req.Id, Name: "restored"}, nil
}

func (s *stubControlSvc) GetAgent(_ context.Context, req *controlv1.GetAgentRequest) (*controlv1.Agent, error) {
	for _, a := range s.agents {
		if a.Id == req.Id {
			return a, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "agent %q not found", req.Id)
}

func (s *stubControlSvc) ListAgents(_ context.Context, _ *controlv1.ListAgentsRequest) (*controlv1.ListAgentsResponse, error) {
	return &controlv1.ListAgentsResponse{Agents: s.agents}, nil
}

func TestAgentGet_gRPC(t *testing.T) {
	stub := &stubControlSvc{
		agents: []*controlv1.Agent{{Id: "agt-1", Name: "researcher"}},
	}
	h := testutil.NewGRPCHarness(t, nil, stub)
	ctx := h.Context()

	out, err := agent.RunGet(ctx, agent.GetInput{Identifier: "agt-1"})
	if err != nil {
		t.Fatalf("RunGet: %v", err)
	}
	if out.Data == nil || out.Data.Id != "agt-1" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestAgentGet_NotFound(t *testing.T) {
	stub := &stubControlSvc{}
	h := testutil.NewGRPCHarness(t, nil, stub)
	ctx := h.Context()

	_, err := agent.RunGet(ctx, agent.GetInput{Identifier: "missing"})
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

func TestAgentList_gRPC(t *testing.T) {
	stub := &stubControlSvc{
		agents: []*controlv1.Agent{
			{Id: "agt-1", Name: "researcher"},
			{Id: "agt-2", Name: "writer"},
		},
	}
	h := testutil.NewGRPCHarness(t, nil, stub)
	ctx := h.Context()

	out, err := agent.RunList(ctx, agent.ListInput{})
	if err != nil {
		t.Fatalf("RunList: %v", err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(out.Items))
	}
}

func TestAgentDelete_gRPC(t *testing.T) {
	stub := &stubControlSvc{agents: []*controlv1.Agent{{Id: "agt-1"}}}
	h := testutil.NewGRPCHarness(t, nil, stub)
	out, err := agent.RunDelete(h.Context(), agent.DeleteInput{ID: "agt-1"})
	if err != nil {
		t.Fatalf("RunDelete: %v", err)
	}
	if !out.Deleted || len(stub.deletedIDs) != 1 || stub.deletedIDs[0] != "agt-1" {
		t.Fatalf("unexpected: out=%+v deleted=%v", out, stub.deletedIDs)
	}
}

func TestAgentDelete_NotFound(t *testing.T) {
	h := testutil.NewGRPCHarness(t, nil, &stubControlSvc{})
	if _, err := agent.RunDelete(h.Context(), agent.DeleteInput{ID: "missing"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestAgentRestore_gRPC(t *testing.T) {
	stub := &stubControlSvc{}
	h := testutil.NewGRPCHarness(t, nil, stub)
	out, err := agent.RunRestore(h.Context(), agent.RestoreInput{ID: "agt-9"})
	if err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if out.Data == nil || out.Data.Id != "agt-9" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(stub.restoredIDs) != 1 {
		t.Fatalf("server not called: %v", stub.restoredIDs)
	}
}
