package credential

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RotateInput struct {
}

type RotateOutput struct {
	Data any `json:"data"`
}

func RunRotate(ctx context.Context, in RotateInput) (*RotateOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "credential rotate is not exposed by the HTTP bridge yet", Exit: 1}
}
