package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ReloadInput struct {
}

type ReloadOutput struct {
	Data any `json:"data"`
}

func RunReload(ctx context.Context, in ReloadInput) (*ReloadOutput, error) {
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
	resp, err := svc.ConfigReload(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &ReloadOutput{Data: resp}, nil
}
