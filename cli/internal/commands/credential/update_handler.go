package credential

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type UpdateInput struct {
	ID          string
	Label       *string
	Description *string
	AccountID   *string
}

type UpdateOutput struct {
	Data *controlv1.ConnectorCredential `json:"data"`
}

func RunUpdate(ctx context.Context, in UpdateInput) (*UpdateOutput, error) {
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
	rec, err := svc.UpdateCredential(ctx, &controlv1.UpdateCredentialRequest{
		Id:     in.ID,
		Update: &controlv1.CredentialUpdate{Label: in.Label, Description: in.Description, AccountId: in.AccountID},
	})
	if err != nil {
		return nil, err
	}
	return &UpdateOutput{Data: rec}, nil
}
