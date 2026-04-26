package agent

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type RestoreInput struct {
	ID string
}

type RestoreOutput struct {
	Data *controlv1.Agent `json:"data"`
}

func RunRestore(ctx context.Context, in RestoreInput) (*RestoreOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	t, err := rt.GetTransport()
	if err != nil {
		return nil, err
	}
	svc, err := t.ControlSvc()
	if err != nil {
		return nil, err
	}
	rec, err := svc.RestoreAgent(ctx, &controlv1.RestoreAgentRequest{Id: in.ID})
	if err != nil {
		return nil, err
	}
	return &RestoreOutput{Data: rec}, nil
}
