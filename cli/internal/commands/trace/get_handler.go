package trace

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
)

type GetInput struct {
	ID string
}

type GetOutput struct {
	Data *runtimev1.RunRecord `json:"data"`
}

func RunGet(ctx context.Context, in GetInput) (*GetOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.RuntimeSvc()
	if err != nil {
		return nil, err
	}
	rec, err := svc.GetRun(ctx, &runtimev1.GetRunRequest{Id: in.ID})
	if err != nil {
		return nil, err
	}
	return &GetOutput{Data: rec}, nil
}
