package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type ValidateInput struct {
}

type ValidateOutput struct {
	Data any `json:"data"`
}

func RunValidate(ctx context.Context, in ValidateInput) (*ValidateOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	if rt.Resolution.LoadedConfig == nil {
		return &ValidateOutput{Data: map[string]any{"valid": true}}, nil
	}
	return &ValidateOutput{Data: map[string]any{"valid": true, "config_path": rt.Resolution.ConfigPath}}, nil
}
