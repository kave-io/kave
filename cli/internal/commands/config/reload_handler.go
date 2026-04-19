package config

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ReloadInput struct {
}

type ReloadOutput struct {
	Data map[string]any `json:"data"`
}

func RunReload(ctx context.Context, in ReloadInput) (*ReloadOutput, error) {
	return nil, output.NotImplemented("config reload")
}
