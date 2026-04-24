package agent

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type ExportInput struct {
	ID string
}

type ExportOutput struct {
	Data any `json:"data"`
}

func RunExport(ctx context.Context, in ExportInput) (*ExportOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	var out ExportOutput
	if err := rt.Client().Get(ctx, "/api/v1/agents/"+in.ID, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
