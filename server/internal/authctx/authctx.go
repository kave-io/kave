package authctx

import "context"

type key struct{}

// Principal is the authenticated caller identity.
type Principal struct {
	Kind    string
	OrgID   string
	UserID  string
	AgentID string
	TokenID string
	Scopes  []string
}

// FromContext extracts a Principal from context.
func FromContext(ctx context.Context) (Principal, bool) {
	v := ctx.Value(key{})
	if v == nil {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}

// WithPrincipal stores a Principal in context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, key{}, p)
}
