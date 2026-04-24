package credential

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RevokeInput struct {
}

type RevokeOutput struct {
	Data any `json:"data"`
}

func RunRevoke(ctx context.Context, in RevokeInput) (*RevokeOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "credential revoke is not exposed by the HTTP bridge yet", Exit: 1}
}
