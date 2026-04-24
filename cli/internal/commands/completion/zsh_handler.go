package completion

import (
	"context"
)

type ZshInput struct {
}

type ZshOutput struct {
	Data any `json:"data"`
}

func RunZsh(ctx context.Context, in ZshInput) (*ZshOutput, error) {
	return &ZshOutput{Data: map[string]any{"shell": "zsh"}}, nil
}
