package price

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
)

type ListInput struct{}

type ListOutput struct {
	Version string                 `json:"version"`
	Entries []*runtimev1.PriceModel `json:"entries"`
}

func RunList(ctx context.Context, _ ListInput) (*ListOutput, error) {
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
	pb, err := svc.GetPriceBook(ctx, &runtimev1.GetPriceBookRequest{})
	if err != nil {
		return nil, err
	}
	return &ListOutput{Version: pb.GetVersion(), Entries: pb.GetEntries()}, nil
}
