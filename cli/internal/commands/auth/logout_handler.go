package auth

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type LogoutInput struct {
}

type LogoutOutput struct {
	Data any `json:"data"`
}

func RunLogout(ctx context.Context, in LogoutInput) (*LogoutOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	client := rt.Client()
	if err := client.Post(ctx, "/api/v1/auth/logout", nil, nil, &map[string]any{}); err != nil {
		return nil, err
	}
	_ = client.ClearSessionToken()
	return &LogoutOutput{Data: map[string]any{"status": "ok"}}, nil
}
