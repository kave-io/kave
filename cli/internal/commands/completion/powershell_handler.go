package completion

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type PowershellInput struct {
}

type PowershellOutput struct {
	Data map[string]any `json:"data"`
}

func RunPowershell(ctx context.Context, in PowershellInput) (*PowershellOutput, error) {
	return nil, output.NotImplemented("completion powershell")
}
