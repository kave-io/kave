package auth

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type WhoamiInput struct {
}

type WhoamiOutput struct {
	Data any `json:"data"`
}

func RunWhoami(ctx context.Context, in WhoamiInput) (*WhoamiOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	client := rt.Client()
	var out any
	if err := client.Get(ctx, "/api/v1/auth/whoami", nil, &out); err != nil {
		return nil, err
	}
	return &WhoamiOutput{Data: out}, nil
}
