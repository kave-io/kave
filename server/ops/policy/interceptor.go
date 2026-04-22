package policy

import (
	"context"
	"errors"

	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pipeline"
	"github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/core/store"
)

var ErrPolicyBlocked = errors.New("gateway policy blocked")

// BlockedError carries the policy decision detail.
type BlockedError struct {
	Reason  string
	Subject string
	Object  string
}

func (e *BlockedError) Error() string {
	if e == nil {
		return ErrPolicyBlocked.Error()
	}
	if e.Reason != "" {
		return e.Reason
	}
	return ErrPolicyBlocked.Error()
}

func (e *BlockedError) Unwrap() error { return ErrPolicyBlocked }

// Interceptor enforces the stored policy before execution begins.
type Interceptor struct {
	store store.AppStore
}

func New(app store.AppStore) *Interceptor {
	return &Interceptor{store: app}
}

func (i *Interceptor) Before(ctx context.Context, action *runtime.Action) (*runtime.Action, error) {
	if i == nil || i.store == nil || action == nil || action.AgentID == "" {
		return action, nil
	}

	pol, err := i.store.GetAgentPolicy(ctx, action.AgentID)
	if err != nil || pol == nil || pol.Status != string(controlmodel.PolicyStatusActive) {
		return action, nil
	}

	if !matchesAny(pol.AllowedTypes, string(action.Type)) {
		return i.block(action, "action type denied", action.Connector+"."+action.Method)
	}
	if !matchesAny(pol.AllowedConnectors, action.Connector) {
		return i.block(action, "connector denied", action.Connector+"."+action.Method)
	}
	if !matchesAny(pol.AllowedMethods, action.Method) {
		return i.block(action, "method denied", action.Connector+"."+action.Method)
	}

	return action, nil
}

func (i *Interceptor) After(context.Context, *runtime.Action, *pipeline.Result) error {
	return nil
}

func (i *Interceptor) Name() string { return "policy" }

func (i *Interceptor) block(action *runtime.Action, reason, object string) (*runtime.Action, error) {
	action.Status = runtime.StatusBlocked
	action.Outcome = &runtime.Outcome{
		Code:    "gateway.policy_blocked",
		Message: reason,
		Reason:  reason,
	}
	return nil, &BlockedError{
		Reason:  reason,
		Subject: action.AgentID,
		Object:  object,
	}
}

func matchesAny(list []string, target string) bool {
	for _, v := range list {
		if v == "*" || v == target {
			return true
		}
	}
	return false
}

var _ pipeline.Interceptor = (*Interceptor)(nil)
