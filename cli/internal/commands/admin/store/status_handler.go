package store

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type StatusInput struct {
}

type StatusOutput struct {
	Data any `json:"data"`
}

func RunStatus(ctx context.Context, in StatusInput) (*StatusOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	var out any
	if err := rt.Client().Get(ctx, "/api/v1/admin/store", nil, &out); err != nil {
		return nil, err
	}
	return &StatusOutput{Data: out}, nil
}
