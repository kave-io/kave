package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type EditInput struct {
}

type EditOutput struct {
	Data any `json:"data"`
}

func RunEdit(ctx context.Context, in EditInput) (*EditOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	return &EditOutput{Data: map[string]any{
		"config_path": rt.Resolution.ConfigPath,
		"hint":        "open the file in your editor",
	}}, nil
}
