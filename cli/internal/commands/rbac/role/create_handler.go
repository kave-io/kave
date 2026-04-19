package role

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type CreateInput struct {
}

type CreateOutput struct {
	Data map[string]any `json:"data"`
}

func RunCreate(ctx context.Context, in CreateInput) (*CreateOutput, error) {
	return nil, output.NotImplemented("role create")
}
