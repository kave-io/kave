package completion

import (
	"context"
)

type PowershellInput struct {
}

type PowershellOutput struct {
	Data any `json:"data"`
}

func RunPowershell(ctx context.Context, in PowershellInput) (*PowershellOutput, error) {
	return &PowershellOutput{Data: map[string]any{"shell": "powershell"}}, nil
}
