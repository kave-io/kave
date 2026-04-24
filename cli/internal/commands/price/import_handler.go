package price

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ImportInput struct {
}

type ImportOutput struct {
	Data any `json:"data"`
}

func RunImport(ctx context.Context, in ImportInput) (*ImportOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "price import is not exposed by the HTTP bridge yet", Exit: 1}
}
