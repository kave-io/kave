package token

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type ListInput struct {
	Agent string
	Page  flags.PageInput
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

	items, nextCursor, err := flags.PaginateAll(ctx, in.Page.All, in.Page.Cursor, 0, func(cursor string) ([]*controlv1.AgentToken, string, error) {
		resp, err := svc.ListTokens(ctx, &controlv1.ListTokensRequest{AgentId: in.Agent, Limit: int32(in.Page.Limit), Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return resp.GetTokens(), resp.GetNextCursor(), nil
	})
	if err != nil {
		return nil, err
	}
	return &ListOutput{Tokens: items, NextCursor: nextCursor}, nil
}
