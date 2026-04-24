package trace

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type TailInput struct {
}

type TailOutput struct {
	Data any `json:"data"`
}

func RunTail(ctx context.Context, in TailInput) (*TailOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "trace tail is not exposed by the HTTP bridge yet", Exit: 1}
}
