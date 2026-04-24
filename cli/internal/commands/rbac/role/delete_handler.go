package role

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type DeleteInput struct {
}

type DeleteOutput struct {
	Data any `json:"data"`
}

func RunDelete(ctx context.Context, in DeleteInput) (*DeleteOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "role delete is not exposed by the HTTP bridge yet", Exit: 1}
}
