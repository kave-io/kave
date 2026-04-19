package completion

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type ZshInput struct {
}

type ZshOutput struct {
	Data map[string]any `json:"data"`
}

func RunZsh(ctx context.Context, in ZshInput) (*ZshOutput, error) {
	return nil, output.NotImplemented("completion zsh")
}
