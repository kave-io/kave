package trace

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/runtime"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
)

type ListInput struct {
	Page    flags.PageInput
	AgentID string
}


type ListOutput struct {
	Items      []*runtimev1.RunRecord `json:"items"`
	NextCursor string                 `json:"next_cursor,omitempty"`
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
	svc, err := t.RuntimeSvc()
	if err != nil {
		return nil, err
	}

	items, nextCursor, err := flags.PaginateAll(ctx, in.Page.All, in.Page.Cursor, 0, func(cursor string) ([]*runtimev1.RunRecord, string, error) {
		filter := &runtimev1.RunFilter{}
		if env := runtime.ActiveEnv(ctx); env != "" {
			filter.EnvId = env
		}
		if in.AgentID != "" {
			filter.AgentId = in.AgentID
		}

		req := &runtimev1.ListRunsRequest{
			Filter: filter,
			Limit:  int32(in.Page.Limit),
			Cursor: cursor,
		}
		resp, err := svc.ListRuns(ctx, req)
		if err != nil {
			return nil, "", err
		}
		return resp.Runs, resp.NextCursor, nil
	})
	if err != nil {
		return nil, err
	}
	return &ListOutput{Items: items, NextCursor: nextCursor}, nil
}
