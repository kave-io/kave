package agent

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/runtime"
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
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	body := map[string]any{}
	if in.Resource.Description != "" {
		body["description"] = &in.Resource.Description
	}
	if in.Policy != "" {
		body["policy_id"] = &in.Policy
	}
	if in.MonthlyBudget != "" {
		body["monthly_budget"] = in.MonthlyBudget
	}
	var out UpdateOutput
	if err := rt.Client().Patch(ctx, "/api/v1/agents/"+in.ID, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
