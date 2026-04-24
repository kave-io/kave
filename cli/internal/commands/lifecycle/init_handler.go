package lifecycle

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type InitInput struct {
}

type InitOutput struct {
	Data any `json:"data"`
}

func RunInit(ctx context.Context, in InitInput) (*InitOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	return &InitOutput{Data: map[string]any{
		"status":     "ok",
		"config_path": rt.Resolution.ConfigPath,
	}}, nil
}
