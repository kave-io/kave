package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type SetInput struct {
}

type SetOutput struct {
	Data any `json:"data"`
}

func RunSet(ctx context.Context, in SetInput) (*SetOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	var out any
	if err := rt.Client().Get(ctx, "/api/v1/config/view", nil, &out); err != nil {
		return nil, err
	}
	return &SetOutput{Data: map[string]any{"status": "ok", "config": out}}, nil
}
