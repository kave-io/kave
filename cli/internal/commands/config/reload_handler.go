package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
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
	var out any
	if err := rt.Client().Post(ctx, "/api/v1/config/reload", nil, nil, &out); err != nil {
		return nil, err
	}
	return &ReloadOutput{Data: out}, nil
}
