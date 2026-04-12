// Package store defines storage interfaces for the Kave control plane.
// Implementations live in server/store/* packages using SQLite, Postgres, and DuckDB.
package store

import (
	"context"

	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
)

// OrgStore owns organization persistence.
type OrgStore interface {
	CreateOrg(ctx context.Context, o *controlmodel.Organization) error
	GetOrg(ctx context.Context, id string) (*controlmodel.Organization, error)
	GetOrgBySlug(ctx context.Context, slug string) (*controlmodel.Organization, error)
}

// UserStore owns user persistence.
type UserStore interface {
	CreateUser(ctx context.Context, u *controlmodel.User) error
	GetUser(ctx context.Context, id string) (*controlmodel.User, error)
	GetUserByEmail(ctx context.Context, orgID, email string) (*controlmodel.User, error)
	UpdateUser(ctx context.Context, id string, update *controlmodel.UserUpdate) error
}

// MembershipStore owns org membership persistence.
type MembershipStore interface {
	AddMember(ctx context.Context, m *controlmodel.Membership) error
	GetMembership(ctx context.Context, orgID, userID string) (*controlmodel.Membership, error)
	ListMembers(ctx context.Context, orgID string) ([]*controlmodel.Membership, error)
	RemoveMember(ctx context.Context, orgID, userID string) error
}

// ProjectStore owns project persistence.
type ProjectStore interface {
	CreateProject(ctx context.Context, p *controlmodel.Project) error
	GetProject(ctx context.Context, id string) (*controlmodel.Project, error)
	ListProjects(ctx context.Context, orgID string) ([]*controlmodel.Project, error)
}

// EnvironmentStore owns environment persistence.
type EnvironmentStore interface {
	CreateEnvironment(ctx context.Context, e *controlmodel.Environment) error
	GetEnvironment(ctx context.Context, id string) (*controlmodel.Environment, error)
	GetEnvironmentBySlug(ctx context.Context, projectID, slug string) (*controlmodel.Environment, error)
	ListEnvironments(ctx context.Context, projectID string) ([]*controlmodel.Environment, error)
}

// AgentStore owns agent persistence.
type AgentStore interface {
	CreateAgent(ctx context.Context, a *controlmodel.Agent) error
	GetAgentByID(ctx context.Context, id string) (*controlmodel.Agent, error)
	GetAgentByName(ctx context.Context, envID, name string) (*controlmodel.Agent, error)
	UpdateAgent(ctx context.Context, id string, update *controlmodel.AgentUpdate) error
	ListAgents(ctx context.Context, envID string) ([]*controlmodel.Agent, error)
}

// PolicyStore owns policy persistence.
type PolicyStore interface {
	CreatePolicy(ctx context.Context, p *controlmodel.PolicyRecord) error
	GetPolicy(ctx context.Context, id string) (*controlmodel.PolicyRecord, error)
	GetAgentPolicy(ctx context.Context, agentID string) (*controlmodel.PolicyRecord, error)
	ListPolicies(ctx context.Context, envID string) ([]*controlmodel.PolicyRecord, error)
}

// RunStore owns run persistence.
type RunStore interface {
	CreateRun(ctx context.Context, r *runtimemodel.RunRecord) error
	GetRunByID(ctx context.Context, id string) (*runtimemodel.RunRecord, error)
	GetRunByIdempotencyKey(ctx context.Context, envID, key string) (*runtimemodel.RunRecord, error)
	UpdateRun(ctx context.Context, id string, update *runtimemodel.RunUpdate) error
	ListRuns(ctx context.Context, filter *runtimemodel.RunFilter) ([]*runtimemodel.RunRecord, error)
}

// ActionStore owns action persistence.
type ActionStore interface {
	CreateAction(ctx context.Context, a *runtimemodel.ActionRecord) error
	GetAction(ctx context.Context, id string) (*runtimemodel.ActionRecord, error)
	ListActionsByRun(ctx context.Context, runID string) ([]*runtimemodel.ActionRecord, error)
}

// CostStore owns price book and budget ledger persistence.
type CostStore interface {
	GetPriceBook(ctx context.Context) (*runtimemodel.PriceBook, error)
	SavePriceBook(ctx context.Context, book *runtimemodel.PriceBook) error
	InsertBudgetEntry(ctx context.Context, entry *runtimemodel.BudgetEntry) error
	AddRunSpend(ctx context.Context, runID string, cost money.Amount) error
	SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (money.Amount, error)
	GetSpendReport(ctx context.Context, filter *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error)
}

// TokenStore owns agent token issuance and lifecycle.
// Stores only the hash of the raw token; raw token is shown once at creation then never persisted.
type TokenStore interface {
	// Issuance
	InsertAgentToken(ctx context.Context, token *controlmodel.AgentToken) error

	// Lookup by hash (never by raw token)
	GetTokenByHash(ctx context.Context, hash string) (*controlmodel.AgentToken, error)

	// Lifecycle
	RevokeToken(ctx context.Context, tokenID, revokedBy, reason string) error
	TouchToken(ctx context.Context, tokenID string) error // updates last_used_at; async-safe

	// Legacy: deny-list for fast JWT rejection (optional optimization, internal to store implementation)
	// Deprecated: use RevokedAt field on AgentToken instead.
	IsTokenRevoked(ctx context.Context, tokenID string) (bool, error)
	InsertRevokedToken(ctx context.Context, tokenID string) error
}

// CredentialStore owns outbound connector secret persistence.
// Supports all four tiers: external reference, encrypted local, ephemeral, pass-through.
type CredentialStore interface {
	// CRUD
	GetCredential(ctx context.Context, id string) (*controlmodel.ConnectorCredential, error)
	StoreCredential(ctx context.Context, c *controlmodel.ConnectorCredential) error
	DeleteCredential(ctx context.Context, id string) error
	ListCredentials(ctx context.Context, envID string) ([]*controlmodel.ConnectorCredential, error)

	// Lookup: policy-driven resolution (not singleton by connector).
	// Fallback chain: exact label match → "primary" label → any active credential for this connector.
	// Returns ErrNoCredential if nothing found — caller decides whether to fall through to passthrough.
	ResolveCredential(ctx context.Context, filter *controlmodel.CredentialFilter) (*controlmodel.ConnectorCredential, error)

	// Lifecycle mutations
	RotateCredential(ctx context.Context, id string, newBlob []byte, wrappingKeyID, rotatedBy string) error
	RevokeCredential(ctx context.Context, id string, revokedBy, reason string) error
	TouchCredential(ctx context.Context, id string) error // updates last_used_at; async-safe
}

// StoreLifecycle owns transactional and migration behavior.
type StoreLifecycle interface {
	WithTx(ctx context.Context, fn func(AppStore) error) error
	Migrate(ctx context.Context) error
	Close() error
}

// AppStore is the primary application data store.
type AppStore interface {
	OrgStore
	UserStore
	MembershipStore
	ProjectStore
	EnvironmentStore
	AgentStore
	PolicyStore
	RunStore
	ActionStore
	CostStore
	TokenStore
	CredentialStore
	StoreLifecycle
}

// SpanStore holds trace spans. Separated from AppStore because it uses a
// different backend optimized for append-heavy analytical queries (DuckDB by default).
type SpanStore interface {
	OpenSpan(ctx context.Context, span *runtimemodel.SpanRow) error
	CloseSpan(ctx context.Context, spanID string, end *runtimemodel.SpanEnd) error
	GetSpan(ctx context.Context, spanID string) (*runtimemodel.SpanRow, error)
	QuerySpans(ctx context.Context, filter *runtimemodel.SpanFilter) ([]*runtimemodel.SpanRow, error)
	SpendByDimension(ctx context.Context, groupBy string, filter *runtimemodel.SpanFilter) (map[string]money.Amount, error)
	Migrate(ctx context.Context) error
	Close() error
}

// AuditStore holds append-only audit logs.
// Separated from AppStore: different backend may be preferred for compliance, and the
// write-only append pattern is distinct from the mutable control-plane data.
type AuditStore interface {
	AppendAudit(ctx context.Context, entry *controlmodel.AuditLog) error
	QueryAudits(ctx context.Context, filter *controlmodel.AuditFilter) ([]*controlmodel.AuditLog, error)
	Migrate(ctx context.Context) error
	Close() error
}
