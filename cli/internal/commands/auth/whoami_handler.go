package auth

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type WhoamiInput struct {
}

type WhoamiOutput struct {
	Data map[string]any `json:"data"`
}

func RunWhoami(ctx context.Context, in WhoamiInput) (*WhoamiOutput, error) {
	return nil, output.NotImplemented("auth whoami")
}
