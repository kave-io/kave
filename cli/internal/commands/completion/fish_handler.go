package completion

import (
	"context"
)

type FishInput struct {
}

type FishOutput struct {
	Data any `json:"data"`
}

func RunFish(ctx context.Context, in FishInput) (*FishOutput, error) {
	return &FishOutput{Data: map[string]any{"shell": "fish"}}, nil
}
