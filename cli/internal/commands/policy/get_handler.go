package policy

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
	return nil, &output.CommandError{Code: "command.unavailable", Message: "policy get is not exposed by the HTTP bridge yet", Exit: 1}
}
