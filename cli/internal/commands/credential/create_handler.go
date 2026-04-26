package credential

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type CreateInput struct {
	EnvID         string
	ConnectorType string
	Label         string
	Secret        string // raw secret bytes; server encrypts at rest when configured
}

type CreateOutput struct {
	Data *controlv1.ConnectorCredential `json:"data"`
}

func RunCreate(ctx context.Context, in CreateInput) (*CreateOutput, error) {
	if in.EnvID == "" || in.ConnectorType == "" {
		return nil, fmt.Errorf("--env and --connector-type are required")
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
	rec, err := svc.CreateCredential(ctx, &controlv1.CreateCredentialRequest{
		EnvId:         in.EnvID,
		ConnectorType: in.ConnectorType,
		Label:         in.Label,
		EncryptedBlob: []byte(in.Secret),
	})
	if err != nil {
		return nil, err
	}
	return &CreateOutput{Data: rec}, nil
}
