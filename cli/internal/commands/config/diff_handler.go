package config

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type DiffInput struct {
}

type DiffOutput struct {
	Data map[string]any `json:"data"`
}

func RunDiff(ctx context.Context, in DiffInput) (*DiffOutput, error) {
	return nil, output.NotImplemented("config diff")
}
