package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
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
	var out any
	if err := rt.Client().Get(ctx, "/api/v1/config/diff", nil, &out); err != nil {
		return nil, err
	}
	return &DiffOutput{Data: out}, nil
}
