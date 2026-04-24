package store

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type MigrateInput struct {
}

type MigrateOutput struct {
	Data any `json:"data"`
}

func RunMigrate(ctx context.Context, in MigrateInput) (*MigrateOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	var out any
	if err := rt.Client().Post(ctx, "/api/v1/admin/store", nil, nil, &out); err != nil {
		return nil, err
	}
	return &MigrateOutput{Data: map[string]any{"status": "ok", "result": out}}, nil
}
