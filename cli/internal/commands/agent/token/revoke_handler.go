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
	return nil, output.NotImplemented("agent token revoke")
}
