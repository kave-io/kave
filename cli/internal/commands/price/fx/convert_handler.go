package fx

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ConvertInput struct {
}

type ConvertOutput struct {
	Data any `json:"data"`
}

func RunConvert(ctx context.Context, in ConvertInput) (*ConvertOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "fx convert : not yet implemented", Exit: 1}
}
