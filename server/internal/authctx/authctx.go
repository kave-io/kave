package authctx

import "context"

type key struct{}

type Kind string

const (
	KindAnonymous Kind = "anonymous"
	KindUser      Kind = "user"
	KindAgent     Kind = "agent"
	KindGuest     Kind = "guest"
	KindInvalid   Kind = "invalid"
)

// Identity is the authenticated caller identity carried through request
// contexts and transport interceptors.
type Identity struct {
	Kind             Kind
	OrgID            string
	UserID           string
	AgentID          string
	ProjectID        string
	EnvID            string
	SessionID        string
	TokenID          string
	Scopes           []string
	Connectors       []string
	Methods          []string
	RawAuthorization string
	ConnectorName    string // for KindGuest: the LLM connector name (e.g., "ollama")
	BindScope        string // for KindGuest: "loopback" or "public"
	Legacy           bool
	Err              string
}

func (i Identity) IsAnonymous() bool  { return i.Kind == "" || i.Kind == KindAnonymous }
func (i Identity) IsUser() bool       { return i.Kind == KindUser }
func (i Identity) IsAgentToken() bool { return i.Kind == KindAgent }
func (i Identity) IsGuest() bool      { return i.Kind == KindGuest }
func (i Identity) IsInvalid() bool    { return i.Kind == KindInvalid }

func (i Identity) Subject() string {
	switch {
	case i.IsUser() && i.UserID != "":
		return "user:" + i.UserID
	case i.IsAgentToken() && i.AgentID != "":
		return "agent:" + i.AgentID
	case i.IsGuest() && i.ConnectorName != "":
		return "guest:" + i.ConnectorName
	case i.OrgID != "":
		return "org:" + i.OrgID
	default:
		return "anonymous"
	}
}

// NewGuest creates a synthetic guest identity.
func NewGuest(envID, orgID, connector, bindScope string) Identity {
	return Identity{
		Kind:          KindGuest,
		EnvID:         envID,
		OrgID:         orgID,
		ConnectorName: connector,
		BindScope:     bindScope,
	}
}

// From extracts the current identity from context.
func From(ctx context.Context) (Identity, bool) {
	v := ctx.Value(key{})
	if v == nil {
		return Identity{}, false
	}
	id, ok := v.(Identity)
	return id, ok
}

// With stores an identity in context.
func With(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, key{}, id)
}

// Compatibility aliases for older call sites.
type Principal = Identity

func FromContext(ctx context.Context) (Principal, bool)              { return From(ctx) }
func WithPrincipal(ctx context.Context, p Principal) context.Context { return With(ctx, p) }
