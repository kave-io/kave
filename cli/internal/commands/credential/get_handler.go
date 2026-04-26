package credential

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type GetInput struct {
	ID string
}

type GetOutput struct {
	Data *controlv1.ConnectorCredential `json:"data"`
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
	svc, err := t.ControlSvc()
	if err != nil {
		return nil, err
	}
	rec, err := svc.GetCredential(ctx, &controlv1.GetCredentialRequest{Id: in.ID})
	if err != nil {
		return nil, err
	}
	return &GetOutput{Data: rec}, nil
}
