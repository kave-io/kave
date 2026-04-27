package connector

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ListInput struct {
}

type ListOutput struct {
	Data any `json:"data"`
}

func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "connector list : not yet implemented", Exit: 1}
}
