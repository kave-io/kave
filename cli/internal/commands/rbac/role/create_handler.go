package role

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type CreateInput struct {
}

type CreateOutput struct {
	Data any `json:"data"`
}

func RunCreate(ctx context.Context, in CreateInput) (*CreateOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "role create is not exposed by the HTTP bridge yet", Exit: 1}
}
