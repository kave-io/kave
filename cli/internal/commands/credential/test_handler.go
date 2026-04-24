package credential

import (
	"context"

	"github.com/kave-io/kave/cli/internal/output"
)

type TestInput struct {
}

type TestOutput struct {
	Data any `json:"data"`
}

func RunTest(ctx context.Context, in TestInput) (*TestOutput, error) {
	return nil, &output.CommandError{Code: "command.unavailable", Message: "credential test is not exposed by the HTTP bridge yet", Exit: 1}
}
