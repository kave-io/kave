package agent

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type GetInput struct {
	Identifier string
}

type GetOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func RunGet(ctx context.Context, in GetInput) (*GetOutput, error) {
	return nil, output.NotImplemented("agent get")
}
