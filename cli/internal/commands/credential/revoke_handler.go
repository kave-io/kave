package credential

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type RevokeInput struct {
	ID     string
	Reason string
}

type RevokeOutput struct {
	ID      string `json:"id"`
	Revoked bool   `json:"revoked"`
}

func RunRevoke(ctx context.Context, in RevokeInput) (*RevokeOutput, error) {
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
	if _, err := svc.RevokeCredential(ctx, &controlv1.RevokeCredentialRequest{Id: in.ID, Reason: in.Reason}); err != nil {
		return nil, err
	}
	return &RevokeOutput{ID: in.ID, Revoked: true}, nil
}
