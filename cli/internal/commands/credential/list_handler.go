package credential

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type ListInput struct {
	Page          flags.PageInput
	ConnectorType string
}

type ListOutput struct {
	Items      []*controlv1.ConnectorCredential `json:"items"`
	NextCursor string                           `json:"next_cursor,omitempty"`
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

	items, nextCursor, err := flags.PaginateAll(ctx, in.Page.All, in.Page.Cursor, 0, func(cursor string) ([]*controlv1.ConnectorCredential, string, error) {
		req := &controlv1.ListCredentialsRequest{
			Limit:  int32(in.Page.Limit),
			Cursor: cursor,
		}
		if in.ConnectorType != "" {
			req.Filter = &controlv1.CredentialFilter{ConnectorType: in.ConnectorType}
		}
		resp, err := svc.ListCredentials(ctx, req)
		if err != nil {
			return nil, "", err
		}
		return resp.Credentials, resp.NextCursor, nil
	})
	if err != nil {
		return nil, err
	}
	return &ListOutput{Items: items, NextCursor: nextCursor}, nil
}
