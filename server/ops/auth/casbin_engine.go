package auth

import (
	"context"
	"fmt"

	coreAuth "github.com/kave-io/kave/core/ops/auth"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/runtime/policy"
	infraAuth "github.com/kave-io/kave/server/internal/infra/casbin"
)

// PolicyEvaluator implements coreAuth.PolicyEngine and pipeline.Interceptor.
type PolicyEvaluator struct {
	casbin infraAuth.Casbin
}

func New(casbin infraAuth.Casbin) *PolicyEvaluator {
	return &PolicyEvaluator{casbin: casbin}
}

// Evaluate checks if the action is allowed under the given auth policy.
func (e *PolicyEvaluator) Evaluate(ctx context.Context, action *runtime.Action, authPolicy *policy.AuthPolicy) (*coreAuth.Decision, error) {
	if authPolicy == nil {
		return &coreAuth.Decision{Allowed: true, Code: "no_policy", Reason: "no auth policy configured"}, nil
	}

	if !matchesAny(authPolicy.AllowedTypes, string(action.Type)) {
		return &coreAuth.Decision{
			Allowed:  false,
			Reason:   fmt.Sprintf("action type %q not in allowed list", action.Type),
			PolicyID: &authPolicy.PolicyID,
			Code:     "type_denied",
		}, nil
	}
	if !matchesAny(authPolicy.AllowedConnectors, action.Connector) {
		return &coreAuth.Decision{
			Allowed:  false,
			Reason:   fmt.Sprintf("connector %q not in allowed list", action.Connector),
			PolicyID: &authPolicy.PolicyID,
			Code:     "connector_denied",
		}, nil
	}
	if !matchesAny(authPolicy.AllowedMethods, action.Method) {
		return &coreAuth.Decision{
			Allowed:  false,
			Reason:   fmt.Sprintf("method %q not in allowed list", action.Method),
			PolicyID: &authPolicy.PolicyID,
			Code:     "method_denied",
		}, nil
	}

	return &coreAuth.Decision{
		Allowed:  true,
		Reason:   "allowed by policy",
		PolicyID: &authPolicy.PolicyID,
		Code:     "allowed",
	}, nil
}

// Before enforces auth policy before the action executes.
func (e *PolicyEvaluator) Before(ctx context.Context, action *runtime.Action) (*runtime.Action, error) {
	p := runtime.PolicyFrom(ctx)
	if p == nil || p.Auth == nil {
		return action, nil
	}
	decision, err := e.Evaluate(ctx, action, p.Auth)
	if err != nil {
		return nil, fmt.Errorf("auth evaluate: %w", err)
	}
	if !decision.Allowed {
		action.Status = runtime.StatusBlocked
		action.Outcome = &runtime.Outcome{
			Code:    decision.Code,
			Message: decision.Reason,
			Reason:  decision.Reason,
		}
		return nil, fmt.Errorf("action denied: %s", decision.Reason)
	}
	return action, nil
}

// After is a no-op.
func (e *PolicyEvaluator) After(_ context.Context, _ *runtime.Action, _ *pipeline.Result) error {
	return nil
}

func (e *PolicyEvaluator) Name() string { return "auth" }

func matchesAny(list []string, target string) bool {
	for _, v := range list {
		if v == "*" || v == target {
			return true
		}
	}
	return false
}

var _ coreAuth.PolicyEngine = (*PolicyEvaluator)(nil)
var _ pipeline.Interceptor = (*PolicyEvaluator)(nil)
