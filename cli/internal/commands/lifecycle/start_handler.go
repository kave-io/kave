package lifecycle

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type StartInput struct {
}

type StartOutput struct {
	Data map[string]any `json:"data"`
}

func RunStart(ctx context.Context, in StartInput) (*StartOutput, error) {
	return nil, output.NotImplemented("lifecycle start")
}
