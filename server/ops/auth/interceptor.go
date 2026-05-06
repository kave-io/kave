package auth

import (
	"context"
	"fmt"

	"github.com/kave-io/kave/core/pipeline"
	coreruntime "github.com/kave-io/kave/core/runtime"
	"github.com/kave-io/kave/server/internal/authctx"
	appcasbin "github.com/kave-io/kave/server/internal/infra/casbin"
)

// Interceptor resolves the caller identity and performs coarse authorization
// before policy/budget/trace run.
type Interceptor struct {
	casbin        appcasbin.Casbin
	anonAllowed   bool
	legacyAllowed bool
}

func NewInterceptor(c appcasbin.Casbin, anonAllowed, legacyAllowed bool) *Interceptor {
	return &Interceptor{casbin: c, anonAllowed: anonAllowed, legacyAllowed: legacyAllowed}
}

func (i *Interceptor) Before(ctx context.Context, action *coreruntime.Action) (*coreruntime.Action, error) {
	if action == nil {
		return nil, nil
	}

	id, _ := authctx.From(ctx)
	switch {
	case id.IsInvalid():
		return nil, ErrUnauthenticated
	case id.IsAnonymous():
		if !i.anonAllowed {
			return nil, ErrUnauthenticated
		}
	case id.IsUser():
		if err := i.enforce(ctx, id, action); err != nil {
			return nil, err
		}
	case id.IsAgentToken():
		if action.AgentID == "" {
			action.AgentID = id.AgentID
		}
		if action.ProjectID == "" {
			action.ProjectID = id.ProjectID
		}
		if action.EnvID == "" {
			action.EnvID = id.EnvID
		}
		if !scopeAllows(id.Connectors, action.Connector) || !scopeAllows(id.Methods, action.Method) {
			return nil, &ErrUnauthorizedError{
				Subject: id.Subject(),
				Object:  action.Connector + "." + action.Method,
				Reason:  "agent token scope denied " + action.Connector + "." + action.Method,
			}
		}
	default:
		if !i.anonAllowed {
			return nil, ErrUnauthenticated
		}
	}

	return action, nil
}

func scopeAllows(scope []string, target string) bool {
	if len(scope) == 0 {
		return true
	}
	for _, value := range scope {
		if value == "*" || value == target {
			return true
		}
	}
	return false
}

func (i *Interceptor) After(context.Context, *coreruntime.Action, *pipeline.Result) error {
	return nil
}

func (i *Interceptor) Name() string { return "auth" }

func (i *Interceptor) enforce(ctx context.Context, id authctx.Identity, action *coreruntime.Action) error {
	if i == nil || i.casbin == nil {
		return nil
	}
	subject := id.Subject()
	if subject == "" {
		return ErrUnauthenticated
	}
	domain := id.OrgID
	if domain == "" {
		domain = "*"
	}
	object := action.Connector + "." + action.Method
	allowed, err := i.casbin.Raw().Enforce(subject, domain, object, action.Method)
	if err != nil {
		return err
	}
	if !allowed {
		return &ErrUnauthorizedError{
			Subject: subject,
			Object:  object,
			Reason:  fmt.Sprintf("casbin denied %s %s", subject, object),
		}
	}
	return nil
}

type ErrUnauthorizedError struct {
	Subject string
	Object  string
	Reason  string
}

func (e *ErrUnauthorizedError) Error() string {
	if e == nil || e.Reason == "" {
		return ErrUnauthorized.Error()
	}
	return e.Reason
}

func (e *ErrUnauthorizedError) Unwrap() error { return ErrUnauthorized }

var _ pipeline.Stage = (*Interceptor)(nil)
