package token

import (
	"context"
	"os"
	"strings"
	"fmt"

	"github.com/kave-io/kave/cli/internal/runtime"
)

type CreateInput struct {
}

type CreateOutput struct {
	Data any `json:"data"`
}

func RunCreate(ctx context.Context, in CreateInput) (*CreateOutput, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok || rt == nil {
		return nil, fmt.Errorf("runtime missing")
	}
	name := strings.TrimSpace(os.Getenv("KAVE_TOKEN_NAME"))
	if name == "" {
		name = "cli"
	}
	scopes := []string{}
	if raw := strings.TrimSpace(os.Getenv("KAVE_TOKEN_SCOPES")); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			if v := strings.TrimSpace(s); v != "" {
				scopes = append(scopes, v)
			}
		}
	}
	var out any
	if err := rt.Client().Post(ctx, "/api/v1/auth/tokens", nil, map[string]any{
		"name":   name,
		"scopes": scopes,
	}, &out); err != nil {
		return nil, err
	}
	return &CreateOutput{Data: out}, nil
}
