package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ViewInput struct {
}

type ViewOutput struct {
	Data any `json:"data"`
}

func RunView(ctx context.Context, in ViewInput) (*ViewOutput, error) {
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
	resp, err := svc.ConfigView(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &ViewOutput{Data: resp}, nil
}
