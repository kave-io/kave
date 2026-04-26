package trace_test

import (
	"context"
	"testing"

	"github.com/kave-io/kave/cli/internal/commands/trace"
	"github.com/kave-io/kave/cli/internal/testutil"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stubRuntimeSvc is a minimal RuntimeServiceServer for testing.
type stubRuntimeSvc struct {
	runtimev1.UnimplementedRuntimeServiceServer
	runs []*runtimev1.RunRecord
}

func (s *stubRuntimeSvc) GetRun(_ context.Context, req *runtimev1.GetRunRequest) (*runtimev1.RunRecord, error) {
	for _, r := range s.runs {
		if r.Id == req.Id {
			return r, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "run %q not found", req.Id)
}

func (s *stubRuntimeSvc) ListRuns(_ context.Context, _ *runtimev1.ListRunsRequest) (*runtimev1.ListRunsResponse, error) {
	return &runtimev1.ListRunsResponse{Runs: s.runs}, nil
}

func TestTraceGet_gRPC(t *testing.T) {
	stub := &stubRuntimeSvc{
		runs: []*runtimev1.RunRecord{{Id: "run-1", AgentId: "agent-a"}},
	}
	h := testutil.NewGRPCHarness(t, stub, nil)
	ctx := h.Context()

	out, err := trace.RunGet(ctx, trace.GetInput{ID: "run-1"})
	if err != nil {
		t.Fatalf("RunGet: %v", err)
	}
	if out.Data == nil || out.Data.Id != "run-1" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestTraceGet_NotFound(t *testing.T) {
	stub := &stubRuntimeSvc{}
	h := testutil.NewGRPCHarness(t, stub, nil)
	ctx := h.Context()

	_, err := trace.RunGet(ctx, trace.GetInput{ID: "missing"})
	if err == nil {
		t.Fatal("expected error for missing run")
	}
}

func TestTraceList_gRPC(t *testing.T) {
	stub := &stubRuntimeSvc{
		runs: []*runtimev1.RunRecord{
			{Id: "run-1", AgentId: "agent-a"},
			{Id: "run-2", AgentId: "agent-b"},
		},
	}
	h := testutil.NewGRPCHarness(t, stub, nil)
	ctx := h.Context()

	out, err := trace.RunList(ctx, trace.ListInput{})
	if err != nil {
		t.Fatalf("RunList: %v", err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(out.Items))
	}
}
