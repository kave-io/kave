package config

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ViewInput struct {
}

type ViewOutput struct {
	Data map[string]any `json:"data"`
}

func RunView(ctx context.Context, in ViewInput) (*ViewOutput, error) {
	return nil, output.NotImplemented("config view")
}
