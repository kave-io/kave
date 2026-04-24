package agent

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/flags"
	"github.com/kave-io/kave/cli/internal/runtime"
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
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	project := runtime.ActiveProject(ctx)
	env := in.Env
	if env == "" {
		env = runtime.ActiveEnv(ctx)
	}
	if project == "" || env == "" {
		return nil, fmt.Errorf("project and env are required")
	}
	body := map[string]any{
		"project_id": project,
		"env_id":     env,
		"name":       in.Resource.Name,
	}
	if in.Resource.Description != "" {
		body["description"] = &in.Resource.Description
	}
	if in.Policy != "" {
		body["policy_id"] = &in.Policy
	}
	if in.MonthlyBudget != "" {
		body["monthly_budget"] = in.MonthlyBudget
	}
	var out CreateOutput
	if err := rt.Client().Post(ctx, "/api/v1/agents", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
