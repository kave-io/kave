package token

import (
	"context"
	"fmt"
	"os"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type RevokeInput struct {
}

type RevokeOutput struct {
	Data any `json:"data"`
}

func RunRevoke(ctx context.Context, in RevokeInput) (*RevokeOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	if id := os.Getenv("KAVE_TOKEN_ID"); id != "" {
		var out any
		if err := rt.Client().Delete(ctx, "/api/v1/auth/tokens/"+id, nil, &out); err != nil {
			return nil, err
		}
		return &RevokeOutput{Data: out}, nil
	}
	_ = rt.Client().ClearSessionToken()
	return &RevokeOutput{Data: map[string]any{"status": "ok"}}, nil
}
