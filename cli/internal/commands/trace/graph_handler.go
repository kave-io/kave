package trace

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type GraphInput struct {
}

type GraphOutput struct {
	Data any `json:"data"`
}

func RunGraph(ctx context.Context, in GraphInput) (*GraphOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "trace graph: not yet implemented", Exit: 1}
}
