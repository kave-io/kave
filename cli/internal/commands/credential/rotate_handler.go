package credential

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type RotateInput struct {
}

type RotateOutput struct {
	Data map[string]any `json:"data"`
}

func RunRotate(ctx context.Context, in RotateInput) (*RotateOutput, error) {
	return nil, output.NotImplemented("credential rotate")
}
