package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
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
	var out any
	if err := rt.Client().Get(ctx, "/api/v1/config/view", nil, &out); err != nil {
		return nil, err
	}
	return &ViewOutput{Data: out}, nil
}
