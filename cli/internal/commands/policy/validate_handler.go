package policy

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ValidateInput struct {
}

type ValidateOutput struct {
	Data any `json:"data"`
}

func RunValidate(ctx context.Context, in ValidateInput) (*ValidateOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "policy validate is not exposed by the HTTP bridge yet", Exit: 1}
}
