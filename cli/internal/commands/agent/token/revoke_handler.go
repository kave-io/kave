package token

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type RevokeInput struct {
	TokenID string
	Reason  string
}

type RevokeOutput struct {
	TokenID string `json:"token_id"`
	Revoked bool   `json:"revoked"`
}

func RunRevoke(ctx context.Context, in RevokeInput) (*RevokeOutput, error) {
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
	if _, err := svc.RevokeToken(ctx, &controlv1.RevokeTokenRequest{Id: in.TokenID, Reason: in.Reason}); err != nil {
		return nil, err
	}
	return &RevokeOutput{TokenID: in.TokenID, Revoked: true}, nil
}
