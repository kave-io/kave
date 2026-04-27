package connector

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type DisableInput struct {
}

type DisableOutput struct {
	Data any `json:"data"`
}

func RunDisable(ctx context.Context, in DisableInput) (*DisableOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "connector disable : not yet implemented", Exit: 1}
}
