package rbac

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type GrantInput struct {
}

type GrantOutput struct {
	Data any `json:"data"`
}

func RunGrant(ctx context.Context, in GrantInput) (*GrantOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "rbac grant is not exposed by the HTTP bridge yet", Exit: 1}
}
