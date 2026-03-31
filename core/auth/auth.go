package auth

import (
	"context"

	"github.com/kave-io/kave/core/intercept"
)

// PolicyEngine evaluates whether an action is permitted under a policy.
// Takes primitives so it works for both Action and Event — the caller extracts fields.
// Complex implementations (Casbin, OPA) live in server/infra/.
type PolicyEngine interface {
	Allowed(ctx context.Context, actionType intercept.ActionType, connector, method string, policy *intercept.Policy) (bool, error)
}

// AllowAll is a PolicyEngine that permits everything. Useful for testing
// and as the default engine before a real policy backend is configured.
type AllowAll struct{}

func (AllowAll) Allowed(_ context.Context, _ intercept.ActionType, _, _ string, _ *intercept.Policy) (bool, error) {
	return true, nil
}

// DenyAll is a PolicyEngine that blocks everything. Useful for testing.
type DenyAll struct{}

func (DenyAll) Allowed(_ context.Context, _ intercept.ActionType, _, _ string, _ *intercept.Policy) (bool, error) {
	return false, nil
}
