package credential

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type RotateInput struct {
	ID     string
	Secret string
}

type RotateOutput struct {
	Data *controlv1.ConnectorCredential `json:"data"`
}

func RunRotate(ctx context.Context, in RotateInput) (*RotateOutput, error) {
	if in.ID == "" {
		return nil, fmt.Errorf("credential id required")
	}
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
	rec, err := svc.RotateCredential(ctx, &controlv1.RotateCredentialRequest{Id: in.ID, NewEncryptedBlob: []byte(in.Secret)})
	if err != nil {
		return nil, err
	}
	return &RotateOutput{Data: rec}, nil
}
