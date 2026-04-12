package auth

import (
	"context"

	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
)

// Decision is the structured result of policy evaluation.
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

func (AllowAll) Evaluate(_ context.Context, action *runtime.Action, authPolicy *policy.AuthPolicy) (*Decision, error) {
	var policyID *string
	if authPolicy != nil && authPolicy.PolicyID != "" {
		policyID = &authPolicy.PolicyID
	}
	_ = action
	return &Decision{
		Allowed:  true,
		Reason:   "allowed",
		PolicyID: policyID,
		Code:     "allow_all",
	}, nil
}

// DenyAll is a PolicyEngine that blocks everything.
type DenyAll struct{}

func (DenyAll) Evaluate(_ context.Context, action *runtime.Action, authPolicy *policy.AuthPolicy) (*Decision, error) {
	var policyID *string
	if authPolicy != nil && authPolicy.PolicyID != "" {
		policyID = &authPolicy.PolicyID
	}
	_ = action
	return &Decision{
		Allowed:  false,
		Reason:   "denied",
		PolicyID: policyID,
		Code:     "deny_all",
	}, nil
}
