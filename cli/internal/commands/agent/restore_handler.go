package agent

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RestoreInput struct {
	ID string
}

type RestoreOutput struct {
	ID      string `json:"id"`
	Restored bool  `json:"restored"`
}

func RunRestore(ctx context.Context, in RestoreInput) (*RestoreOutput, error) {
	return nil, output.NotImplemented("agent restore")
}
