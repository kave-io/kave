package ctx

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type ListInput struct {
}

type ListOutput struct {
	Data any `json:"data"`
}

func RunList(ctx context.Context, in ListInput) (*ListOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	contexts := []map[string]any{}
	if rt.Resolution.LoadedConfig != nil {
		for _, c := range rt.Resolution.LoadedConfig.Contexts {
			contexts = append(contexts, map[string]any{
				"name":    c.Name,
				"server":  c.Server,
				"user":    c.User,
				"project": c.Project,
				"env":     c.Env,
			})
		}
	}
	return &ListOutput{Data: map[string]any{
		"current": rt.Resolution.ActiveContextName(),
		"contexts": contexts,
	}}, nil
}
