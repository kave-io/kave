package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	"google.golang.org/protobuf/types/known/emptypb"
)

type DiffInput struct {
}

type DiffOutput struct {
	Data any `json:"data"`
}

func RunDiff(ctx context.Context, in DiffInput) (*DiffOutput, error) {
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
	resp, err := svc.ConfigDiff(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &DiffOutput{Data: resp}, nil
}
