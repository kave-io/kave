package config

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type SetInput struct {
}

type SetOutput struct {
	Data map[string]any `json:"data"`
}

func RunSet(ctx context.Context, in SetInput) (*SetOutput, error) {
	return nil, output.NotImplemented("config set")
}
