package credential

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	controlv1 "github.com/kave-io/kave/proto/gen/kave/control/v1"
)

type DeleteInput struct {
	ID string
}

type DeleteOutput struct{}

func RunDelete(ctx context.Context, in DeleteInput) (*DeleteOutput, error) {
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
	if _, err := svc.DeleteCredential(ctx, &controlv1.DeleteCredentialRequest{Id: in.ID}); err != nil {
		return nil, err
	}
	return &DeleteOutput{}, nil
}
