package config

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type PathInput struct {
}

type PathOutput struct {
	Data any `json:"data"`
}

func RunPath(ctx context.Context, in PathInput) (*PathOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	res := map[string]any{
		"config_path": rt.Resolution.ConfigPath,
		"project_path": rt.Resolution.ProjectPath,
		"user_path":    rt.Resolution.UserPath,
		"system_path":  rt.Resolution.SystemPath,
	}
	if ctx := rt.Resolution.ActiveContext(); ctx != nil {
		res["current_context"] = ctx.Name
		res["server"] = ctx.Server
		res["project"] = ctx.Project
		res["env"] = ctx.Env
	}
	return &PathOutput{Data: res}, nil
}
