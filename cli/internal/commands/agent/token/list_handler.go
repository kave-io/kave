package token

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type ListInput struct {
	Agent  string
	Limit  int32
	Cursor string
}

type ListOutput struct {
	Tokens     []*controlv1.AgentToken `json:"tokens"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
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
	resp, err := svc.ListTokens(ctx, &controlv1.ListTokensRequest{AgentId: in.Agent, Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return nil, err
	}
	return &ListOutput{Tokens: resp.GetTokens(), NextCursor: resp.GetNextCursor()}, nil
}
