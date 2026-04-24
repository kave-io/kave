package ctx

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type UseInput struct {
}

type UseOutput struct {
	Data any `json:"data"`
}

func RunUse(ctx context.Context, in UseInput) (*UseOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	active := rt.Resolution.ActiveContext()
	if active == nil {
		return &UseOutput{Data: map[string]any{"status": "no-context"}}, nil
	}
	return &UseOutput{Data: map[string]any{
		"status":  "ok",
		"context": active.Name,
		"server":  active.Server,
	}}, nil
}
