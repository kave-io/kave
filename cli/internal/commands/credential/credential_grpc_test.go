package credential_test

import (
	"context"
	"testing"

	"github.com/kave-io/kave/cli/internal/commands/credential"
	"github.com/kave-io/kave/cli/internal/testutil"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type stubControlSvc struct {
	controlv1.UnimplementedControlPlaneServiceServer
	creds   []*controlv1.ConnectorCredential
	deleted []string
}

func (s *stubControlSvc) GetCredential(_ context.Context, req *controlv1.GetCredentialRequest) (*controlv1.ConnectorCredential, error) {
	for _, c := range s.creds {
		if c.Id == req.Id {
			return c, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "credential %q not found", req.Id)
}

func (s *stubControlSvc) ListCredentials(_ context.Context, _ *controlv1.ListCredentialsRequest) (*controlv1.ListCredentialsResponse, error) {
	return &controlv1.ListCredentialsResponse{Credentials: s.creds}, nil
}

func (s *stubControlSvc) DeleteCredential(_ context.Context, req *controlv1.DeleteCredentialRequest) (*emptypb.Empty, error) {
	s.deleted = append(s.deleted, req.Id)
	return &emptypb.Empty{}, nil
}

func TestCredentialGet_gRPC(t *testing.T) {
	stub := &stubControlSvc{
		creds: []*controlv1.ConnectorCredential{{Id: "cred-1", ConnectorType: "openai"}},
	}
	h := testutil.NewGRPCHarness(t, nil, stub)
	ctx := h.Context()

	out, err := credential.RunGet(ctx, credential.GetInput{ID: "cred-1"})
	if err != nil {
		t.Fatalf("RunGet: %v", err)
	}
	if out.Data == nil || out.Data.Id != "cred-1" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestCredentialList_gRPC(t *testing.T) {
	stub := &stubControlSvc{
		creds: []*controlv1.ConnectorCredential{
			{Id: "cred-1", ConnectorType: "openai"},
			{Id: "cred-2", ConnectorType: "anthropic"},
		},
	}
	h := testutil.NewGRPCHarness(t, nil, stub)
	ctx := h.Context()

	out, err := credential.RunList(ctx, credential.ListInput{})
	if err != nil {
		t.Fatalf("RunList: %v", err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(out.Items))
	}
}

func TestCredentialDelete_gRPC(t *testing.T) {
	stub := &stubControlSvc{}
	h := testutil.NewGRPCHarness(t, nil, stub)
	ctx := h.Context()

	_, err := credential.RunDelete(ctx, credential.DeleteInput{ID: "cred-1"})
	if err != nil {
		t.Fatalf("RunDelete: %v", err)
	}
	if len(stub.deleted) != 1 || stub.deleted[0] != "cred-1" {
		t.Fatalf("expected deleted=[cred-1], got %v", stub.deleted)
	}
}
