package policy

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type UpdateInput struct {
}

type UpdateOutput struct {
	Data any `json:"data"`
}

func RunUpdate(ctx context.Context, in UpdateInput) (*UpdateOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "policy update is not exposed by the HTTP bridge yet", Exit: 1}
}
