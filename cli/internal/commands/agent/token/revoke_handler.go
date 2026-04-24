package token

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RevokeInput struct {
	TokenID string
}

type RevokeOutput struct {
	TokenID  string `json:"token_id"`
	Revoked  bool   `json:"revoked"`
}

func RunRevoke(ctx context.Context, in RevokeInput) (*RevokeOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "agent token revoke is not exposed by the HTTP bridge yet", Exit: 1}
}
