package completion

import (
	"context"
)

type BashInput struct {
}

type BashOutput struct {
	Data any `json:"data"`
}

func RunBash(ctx context.Context, in BashInput) (*BashOutput, error) {
	return &BashOutput{Data: map[string]any{"shell": "bash"}}, nil
}
