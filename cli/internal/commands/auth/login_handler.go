package auth

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type LoginInput struct {
}

type LoginOutput struct {
	Data map[string]any `json:"data"`
}

func RunLogin(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	return nil, output.NotImplemented("auth login")
}
