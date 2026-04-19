package rbac

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RevokeInput struct {
}

type RevokeOutput struct {
	Data map[string]any `json:"data"`
}

func RunRevoke(ctx context.Context, in RevokeInput) (*RevokeOutput, error) {
	return nil, output.NotImplemented("rbac revoke")
}
