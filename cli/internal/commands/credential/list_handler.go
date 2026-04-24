package credential

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ListInput struct {
}

type ListOutput struct {
	Data any `json:"data"`
}

func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "credential list is not exposed by the HTTP bridge yet", Exit: 1}
}
