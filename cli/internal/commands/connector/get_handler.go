package connector

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type GetInput struct {
}

type GetOutput struct {
	Data any `json:"data"`
}

func RunGet(ctx context.Context, in GetInput) (*GetOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "connector get : not yet implemented", Exit: 1}
}
