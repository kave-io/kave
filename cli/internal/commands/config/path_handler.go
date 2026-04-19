package config

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type PathInput struct {
}

type PathOutput struct {
	Data map[string]any `json:"data"`
}

func RunPath(ctx context.Context, in PathInput) (*PathOutput, error) {
	return nil, output.NotImplemented("config path")
}
