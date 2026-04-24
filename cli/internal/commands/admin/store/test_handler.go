package store

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type TestInput struct {
}

type TestOutput struct {
	Data any `json:"data"`
}

func RunTest(ctx context.Context, in TestInput) (*TestOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	var out any
	if err := rt.Client().Get(ctx, "/api/v1/admin/store", nil, &out); err != nil {
		return nil, err
	}
	return &TestOutput{Data: map[string]any{"status": "ok", "result": out}}, nil
}
