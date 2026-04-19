package trace

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type GraphInput struct {
}

type GraphOutput struct {
	Data map[string]any `json:"data"`
}

func RunGraph(ctx context.Context, in GraphInput) (*GraphOutput, error) {
	return nil, output.NotImplemented("trace graph")
}
