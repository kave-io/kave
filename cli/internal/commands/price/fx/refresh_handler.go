package fx

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RefreshInput struct {
}

type RefreshOutput struct {
	Data any `json:"data"`
}

func RunRefresh(ctx context.Context, in RefreshInput) (*RefreshOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "fx refresh is not exposed by the HTTP bridge yet", Exit: 1}
}
