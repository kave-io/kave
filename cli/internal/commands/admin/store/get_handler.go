package store

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GetInput struct {
}

type GetOutput struct {
	Data any `json:"data"`
}

func RunGet(ctx context.Context, in GetInput) (*GetOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.DaemonSvc()
	if err != nil {
		return nil, err
	}
	resp, err := svc.AdminStore(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &GetOutput{Data: resp}, nil
}
