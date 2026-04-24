package lifecycle

import (
	"context"
	"fmt"

)

type StartInput struct {
}

type StartOutput struct {
	Data any `json:"data"`
}

func RunStart(ctx context.Context, in StartInput) (*StartOutput, error) {
	if ctx == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	return &StartOutput{Data: map[string]any{"status": "ok"}}, nil
}
