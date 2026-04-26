package price

import (
	"context"
	"fmt"
	"strings"

	"github.com/kave-io/kave/cli/internal/runtime"
	runtimev1 "github.com/kave-io/kave/proto/gen/kave/runtime/v1"
)

type GetInput struct {
	Provider string
	Match    string // model name or pattern
}

type GetOutput struct {
	Data *runtimev1.PriceModel `json:"data"`
}

func RunGet(ctx context.Context, in GetInput) (*GetOutput, error) {
	if in.Match == "" {
		return nil, fmt.Errorf("--match (model name) is required")
	}
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
	for _, e := range pb.GetEntries() {
		if in.Provider != "" && !strings.EqualFold(e.GetProvider(), in.Provider) {
			continue
		}
		if e.GetMatch() == in.Match {
			return &GetOutput{Data: e}, nil
		}
	}
	return nil, fmt.Errorf("no price entry for %q", in.Match)
}
