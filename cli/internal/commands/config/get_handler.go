package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
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
	var out any
	if err := rt.Client().Get(ctx, "/api/v1/config/view", nil, &out); err != nil {
		return nil, err
	}
	return &GetOutput{Data: out}, nil
}
