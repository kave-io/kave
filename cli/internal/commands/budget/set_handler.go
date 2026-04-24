package budget

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type SetInput struct {
}

type SetOutput struct {
	Data any `json:"data"`
}

func RunSet(ctx context.Context, in SetInput) (*SetOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "budget set is not exposed by the HTTP bridge yet", Exit: 1}
}
