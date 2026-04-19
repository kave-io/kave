package agent

import (
	"context"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/output"
)

type UpdateInput struct {
	ID             string
	Resource       flags.ResourceInput
	Policy         string
	Credentials    []string
	MonthlyBudget  string
}

type UpdateOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func RunUpdate(ctx context.Context, in UpdateInput) (*UpdateOutput, error) {
	return nil, output.NotImplemented("agent update")
}
