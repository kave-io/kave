package auth

import (
	"context"

	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
)

// Decision is the structured result of policy evaluation.
// PolicyID is optional and typically stamped by the caller from the owning Policy.ID
// (the sub-policy itself no longer carries it).
type Decision struct {
	Allowed  bool
	Reason   string
	PolicyID *string
	Code     string
}

// PolicyEngine evaluates an action under a policy and returns a structured decision.
type PolicyEngine interface {
	Evaluate(ctx context.Context, action *runtime.Action, policy *policy.AuthPolicy) (*Decision, error)
}

// AllowAll is a PolicyEngine that permits everything.
type AllowAll struct{}

func (AllowAll) Evaluate(_ context.Context, _ *runtime.Action, _ *policy.AuthPolicy) (*Decision, error) {
	return &Decision{Allowed: true, Reason: "allowed", Code: "allow_all"}, nil
}

// DenyAll is a PolicyEngine that blocks everything.
type DenyAll struct{}

func (DenyAll) Evaluate(_ context.Context, _ *runtime.Action, _ *policy.AuthPolicy) (*Decision, error) {
	return &Decision{Allowed: false, Reason: "denied", Code: "deny_all"}, nil
}
