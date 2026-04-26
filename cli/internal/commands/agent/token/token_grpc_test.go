package token_test

import (
	"context"
	"testing"

	"github.com/kave-io/kave/cli/internal/commands/agent/token"
	"github.com/kave-io/kave/cli/internal/testutil"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type stubControlSvc struct {
	controlv1.UnimplementedControlPlaneServiceServer
	tokens      []*controlv1.AgentToken
	createdFor  string
	revokedID   string
	revokeError bool
}

func (s *stubControlSvc) CreateToken(_ context.Context, req *controlv1.CreateTokenRequest) (*controlv1.CreateTokenResponse, error) {
	s.createdFor = req.AgentId
	tok := &controlv1.AgentToken{Id: "tok-1", AgentId: req.AgentId, Name: req.Name}
	return &controlv1.CreateTokenResponse{Token: tok, RawToken: "kav_raw"}, nil
}

func (s *stubControlSvc) ListTokens(_ context.Context, req *controlv1.ListTokensRequest) (*controlv1.ListTokensResponse, error) {
	out := []*controlv1.AgentToken{}
	for _, t := range s.tokens {
		if t.AgentId == req.AgentId {
			out = append(out, t)
		}
	}
	return &controlv1.ListTokensResponse{Tokens: out, NextCursor: ""}, nil
}

func (s *stubControlSvc) RevokeToken(_ context.Context, req *controlv1.RevokeTokenRequest) (*emptypb.Empty, error) {
	if s.revokeError {
		return nil, status.Errorf(codes.NotFound, "token %q not found", req.Id)
	}
	s.revokedID = req.Id
	return &emptypb.Empty{}, nil
}

func TestTokenIssue_gRPC(t *testing.T) {
	stub := &stubControlSvc{}
	h := testutil.NewGRPCHarness(t, nil, stub)

	out, err := token.RunIssue(h.Context(), token.IssueInput{Agent: "agt-1", Name: "primary"})
	if err != nil {
		t.Fatalf("RunIssue: %v", err)
	}
	if out.RawToken != "kav_raw" || out.Token == nil || out.Token.AgentId != "agt-1" {
		t.Fatalf("unexpected: %+v", out)
	}
	if stub.createdFor != "agt-1" {
		t.Fatalf("server not called with agent")
	}
}

func TestTokenList_gRPC(t *testing.T) {
	stub := &stubControlSvc{tokens: []*controlv1.AgentToken{
		{Id: "t1", AgentId: "agt-1"},
		{Id: "t2", AgentId: "agt-2"},
	}}
	h := testutil.NewGRPCHarness(t, nil, stub)

	out, err := token.RunList(h.Context(), token.ListInput{Agent: "agt-1"})
	if err != nil {
		t.Fatalf("RunList: %v", err)
	}
	if len(out.Tokens) != 1 || out.Tokens[0].Id != "t1" {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestTokenRevoke_gRPC(t *testing.T) {
	stub := &stubControlSvc{}
	h := testutil.NewGRPCHarness(t, nil, stub)

	out, err := token.RunRevoke(h.Context(), token.RevokeInput{TokenID: "tok-9"})
	if err != nil {
		t.Fatalf("RunRevoke: %v", err)
	}
	if !out.Revoked || stub.revokedID != "tok-9" {
		t.Fatalf("unexpected: %+v %q", out, stub.revokedID)
	}
}

func TestTokenRevoke_NotFound(t *testing.T) {
	stub := &stubControlSvc{revokeError: true}
	h := testutil.NewGRPCHarness(t, nil, stub)
	if _, err := token.RunRevoke(h.Context(), token.RevokeInput{TokenID: "x"}); err == nil {
		t.Fatal("expected error")
	}
}
