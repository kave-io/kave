package credential

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type TestInput struct {
}

type TestOutput struct {
	Data map[string]any `json:"data"`
}

func RunTest(ctx context.Context, in TestInput) (*TestOutput, error) {
	return nil, output.NotImplemented("credential test")
}
