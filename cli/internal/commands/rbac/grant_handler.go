package rbac

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type GrantInput struct {
}

type GrantOutput struct {
	Data map[string]any `json:"data"`
}

func RunGrant(ctx context.Context, in GrantInput) (*GrantOutput, error) {
	return nil, output.NotImplemented("rbac grant")
}
