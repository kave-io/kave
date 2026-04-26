package agent

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type ListInput struct {
	Page flags.PageInput
}

type ListOutput struct {
	Items      []*controlv1.Agent `json:"items"`
	NextCursor string             `json:"next_cursor,omitempty"`
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

	items, nextCursor, err := flags.PaginateAll(ctx, in.Page.All, in.Page.Cursor, 0, func(cursor string) ([]*controlv1.Agent, string, error) {
		req := &controlv1.ListAgentsRequest{
			Limit:  int32(in.Page.Limit),
			Cursor: cursor,
		}
		if env := runtime.ActiveEnv(ctx); env != "" {
			req.EnvId = env
		}
		resp, err := svc.ListAgents(ctx, req)
		if err != nil {
			return nil, "", err
		}
		return resp.Agents, resp.NextCursor, nil
	})
	if err != nil {
		return nil, err
	}
	return &ListOutput{Items: items, NextCursor: nextCursor}, nil
}
