package auth

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type LogoutInput struct {
}

type LogoutOutput struct {
	Data map[string]any `json:"data"`
}

func RunLogout(ctx context.Context, in LogoutInput) (*LogoutOutput, error) {
	return nil, output.NotImplemented("auth logout")
}
