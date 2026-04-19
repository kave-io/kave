package config

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type EditInput struct {
}

type EditOutput struct {
	Data map[string]any `json:"data"`
}

func RunEdit(ctx context.Context, in EditInput) (*EditOutput, error) {
	return nil, output.NotImplemented("config edit")
}
