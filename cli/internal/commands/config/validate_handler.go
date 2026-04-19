package config

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ValidateInput struct {
}

type ValidateOutput struct {
	Data map[string]any `json:"data"`
}

func RunValidate(ctx context.Context, in ValidateInput) (*ValidateOutput, error) {
	return nil, output.NotImplemented("config validate")
}
