// Package authctx carries the already-authenticated V2 service-key identity.
// It is intentionally separate from V1's human/session/agent identity model.
package authctx

import (
	"context"

	v2 "github.com/kave-io/kave/core/v2"
)

type key struct{}

func WithCaller(ctx context.Context, caller v2.Caller) context.Context {
	return context.WithValue(ctx, key{}, caller)
}

func CallerFrom(ctx context.Context) (v2.Caller, bool) {
	caller, ok := ctx.Value(key{}).(v2.Caller)
	return caller, ok
}
