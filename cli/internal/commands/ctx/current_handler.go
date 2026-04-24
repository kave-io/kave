package ctx

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type CurrentInput struct {
}

type CurrentOutput struct {
	Data any `json:"data"`
}

func RunCurrent(ctx context.Context, in CurrentInput) (*CurrentOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	active := rt.Resolution.ActiveContext()
	if active == nil {
		return &CurrentOutput{Data: map[string]any{"current": ""}}, nil
	}
	return &CurrentOutput{Data: map[string]any{
		"current": active.Name,
		"server":  active.Server,
		"user":    active.User,
		"project": active.Project,
		"env":     active.Env,
	}}, nil
}
