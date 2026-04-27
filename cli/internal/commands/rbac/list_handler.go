package rbac

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ListInput struct {
}

type ListOutput struct {
	Data any `json:"data"`
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
	svc, err := t.RBACSvc()
	if err != nil {
		return nil, err
	}
	resp, err := svc.ListBindings(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &ListOutput{Data: resp.Bindings}, nil
}
