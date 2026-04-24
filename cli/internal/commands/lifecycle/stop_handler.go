package lifecycle

import (
	"context"
	"fmt"

)

type StopInput struct {
}

type StopOutput struct {
	Data any `json:"data"`
}

func RunStop(ctx context.Context, in StopInput) (*StopOutput, error) {
	if ctx == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	return &StopOutput{Data: map[string]any{"status": "ok"}}, nil
}
