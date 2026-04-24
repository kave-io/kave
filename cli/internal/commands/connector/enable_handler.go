package connector

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type EnableInput struct {
}

type EnableOutput struct {
	Data any `json:"data"`
}

func RunEnable(ctx context.Context, in EnableInput) (*EnableOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "connector enable is not exposed by the HTTP bridge yet", Exit: 1}
}
