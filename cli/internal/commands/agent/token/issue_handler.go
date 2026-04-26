package token

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type IssueInput struct {
	Agent string
	Name  string
}

type IssueOutput struct {
	Token    *controlv1.AgentToken `json:"token"`
	RawToken string                `json:"raw_token"`
}

func RunIssue(ctx context.Context, in IssueInput) (*IssueOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.ControlSvc()
	if err != nil {
		return nil, err
	}
	resp, err := svc.CreateToken(ctx, &controlv1.CreateTokenRequest{AgentId: in.Agent, Name: in.Name})
	if err != nil {
		return nil, err
	}
	return &IssueOutput{Token: resp.GetToken(), RawToken: resp.GetRawToken()}, nil
}
