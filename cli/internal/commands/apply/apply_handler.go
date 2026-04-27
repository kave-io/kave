package apply

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ApplyInput struct {
}

type ApplyOutput struct {
	Data any `json:"data"`
}

func RunApply(ctx context.Context, in ApplyInput) (*ApplyOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "apply apply : not yet implemented", Exit: 1}
}
