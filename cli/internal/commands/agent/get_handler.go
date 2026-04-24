package agent

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type GetInput struct {
	Identifier string
}

type GetOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func RunGet(ctx context.Context, in GetInput) (*GetOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	var out GetOutput
	if err := rt.Client().Get(ctx, "/api/v1/agents/"+in.Identifier, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
