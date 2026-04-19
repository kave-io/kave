package agent

import (
	"context"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/output"
)

type CreateInput struct {
	Resource       flags.ResourceInput
	Env            string
	Policy         string
	Credentials    []string
	MonthlyBudget  string
}

type CreateOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func RunCreate(ctx context.Context, in CreateInput) (*CreateOutput, error) {
	return nil, output.NotImplemented("agent create")
}
