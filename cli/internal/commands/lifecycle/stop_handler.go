package lifecycle

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type StopInput struct {
}

type StopOutput struct {
	Data map[string]any `json:"data"`
}

func RunStop(ctx context.Context, in StopInput) (*StopOutput, error) {
	return nil, output.NotImplemented("lifecycle stop")
}
