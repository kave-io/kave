// Package store defines storage interfaces for the Kave control plane.
// Implementations live in server/store/* packages using SQLite, Postgres, and DuckDB.
package store

import (
	"context"

	auditmodel "github.com/kave-io/kave/core/model/audit"
	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
)

// OrgStore owns organization persistence.
type OrgStore interface {
	CreateOrg(ctx context.Context, o *controlmodel.Organization) error
	GetOrg(ctx context.Context, id string) (*controlmodel.Organization, error)
	GetOrgBySlug(ctx context.Context, slug string) (*controlmodel.Organization, error)
	ListOrgs(ctx context.Context, page Page) (PageResult[*controlmodel.Organization], error)
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
	ListMembers(ctx context.Context, orgID string, page Page) (PageResult[*controlmodel.Membership], error)
	RemoveMember(ctx context.Context, orgID, userID string) error
}

// ProjectStore owns project persistence.
type ProjectStore interface {
	CreateProject(ctx context.Context, p *controlmodel.Project) error
	GetProject(ctx context.Context, id string) (*controlmodel.Project, error)
	ListProjects(ctx context.Context, orgID string, page Page) (PageResult[*controlmodel.Project], error)
}

// EnvironmentStore owns environment persistence.
type EnvironmentStore interface {
	CreateEnvironment(ctx context.Context, e *controlmodel.Environment) error
	GetEnvironment(ctx context.Context, id string) (*controlmodel.Environment, error)
	GetEnvironmentBySlug(ctx context.Context, projectID, slug string) (*controlmodel.Environment, error)
	ListEnvironments(ctx context.Context, projectID string, page Page) (PageResult[*controlmodel.Environment], error)
}

// AgentStore owns agent persistence.
// DeleteAgent is a soft delete: it sets DeletedAt and excludes the agent from
// default listings. RestoreAgent clears DeletedAt.
type AgentStore interface {
	CreateAgent(ctx context.Context, a *controlmodel.Agent) error
	GetAgentByID(ctx context.Context, id string) (*controlmodel.Agent, error)
	GetAgentByName(ctx context.Context, envID, name string) (*controlmodel.Agent, error)
	UpdateAgent(ctx context.Context, id string, update *controlmodel.AgentUpdate) error
	ListAgents(ctx context.Context, envID string, page Page) (PageResult[*controlmodel.Agent], error)
	DeleteAgent(ctx context.Context, id, deletedBy string) error
	RestoreAgent(ctx context.Context, id, restoredBy string) error
}

// PolicyStore owns policy persistence.
type PolicyStore interface {
	CreatePolicy(ctx context.Context, p *controlmodel.PolicyRecord) error
	GetPolicy(ctx context.Context, id string) (*controlmodel.PolicyRecord, error)
	UpdatePolicy(ctx context.Context, id string, update *controlmodel.PolicyUpdate) error
	DeletePolicy(ctx context.Context, id string) error
	GetAgentPolicy(ctx context.Context, agentID string) (*controlmodel.PolicyRecord, error)
	ListPolicies(ctx context.Context, envID string, page Page) (PageResult[*controlmodel.PolicyRecord], error)
}

// BudgetStore owns per-agent budget persistence.
type BudgetStore interface {
	CreateBudget(ctx context.Context, b *controlmodel.Budget) error
	GetBudget(ctx context.Context, agentID string) (*controlmodel.Budget, error)
	DeleteBudget(ctx context.Context, agentID string) error
}

// RunStore owns run persistence.
type RunStore interface {
	CreateRun(ctx context.Context, r *runtimemodel.RunRecord) error
	GetRunByID(ctx context.Context, id string) (*runtimemodel.RunRecord, error)
	GetRunByIdempotencyKey(ctx context.Context, envID, key string) (*runtimemodel.RunRecord, error)
	UpdateRun(ctx context.Context, id string, update *runtimemodel.RunUpdate) error
	ListRuns(ctx context.Context, filter *runtimemodel.RunFilter, page Page) (PageResult[*runtimemodel.RunRecord], error)
}

// ActionStore owns action persistence.
type ActionStore interface {
	CreateAction(ctx context.Context, a *runtimemodel.ActionRecord) error
	GetAction(ctx context.Context, id string) (*runtimemodel.ActionRecord, error)
	UpdateAction(ctx context.Context, id string, update *runtimemodel.ActionUpdate) error
	ListActionsByRun(ctx context.Context, runID string, page Page) (PageResult[*runtimemodel.ActionRecord], error)
}

// CostStore owns price book and budget ledger persistence.
type CostStore interface {
	GetPriceBook(ctx context.Context) (*runtimemodel.PriceBook, error)
	SavePriceBook(ctx context.Context, book *runtimemodel.PriceBook) error
	ListFXRates(ctx context.Context) ([]runtimemodel.FXRateRecord, error)
	GetFXRate(ctx context.Context, base, quote money.CurrencyCode) (*runtimemodel.FXRateRecord, error)
	UpsertFXRates(ctx context.Context, rates []runtimemodel.FXRateRecord) error
	ListFXCurrencies(ctx context.Context) ([]runtimemodel.FXCurrencyRecord, error)
	UpsertFXCurrencies(ctx context.Context, currencies []runtimemodel.FXCurrencyRecord) error
	InsertBudgetEntry(ctx context.Context, entry *runtimemodel.BudgetEntry) error
	AddRunSpend(ctx context.Context, runID string, cost money.Amount) error
	SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (money.Amount, error)
	GetSpendReport(ctx context.Context, filter *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error)
}

// TokenStore owns agent token issuance and lifecycle.
// Stores only the hash of the raw token; raw token is shown once at creation then never persisted.
type TokenStore interface {
	// Sessions
	InsertSession(ctx context.Context, session *controlmodel.Session) error
	GetSessionByHash(ctx context.Context, hash string) (*controlmodel.Session, error)
	GetSession(ctx context.Context, id string) (*controlmodel.Session, error)
	ListSessions(ctx context.Context, userID string, page Page) (PageResult[*controlmodel.Session], error)
	RevokeSession(ctx context.Context, sessionID, revokedBy string) error
	TouchSession(ctx context.Context, sessionID string) error

	// API tokens
	InsertAPIToken(ctx context.Context, token *controlmodel.APIToken) error
	GetAPITokenByHash(ctx context.Context, hash string) (*controlmodel.APIToken, error)
	GetAPIToken(ctx context.Context, id string) (*controlmodel.APIToken, error)
	ListAPITokens(ctx context.Context, userID string, page Page) (PageResult[*controlmodel.APIToken], error)
	RevokeAPIToken(ctx context.Context, tokenID, revokedBy, reason string) error
	TouchAPIToken(ctx context.Context, tokenID string) error

	// Agent tokens
	InsertAgentToken(ctx context.Context, token *controlmodel.AgentToken) error
	GetAgentTokenByHash(ctx context.Context, hash string) (*controlmodel.AgentToken, error)
	GetAgentToken(ctx context.Context, id string) (*controlmodel.AgentToken, error)
	ListAgentTokens(ctx context.Context, agentID string, page Page) (PageResult[*controlmodel.AgentToken], error)
	RevokeAgentToken(ctx context.Context, tokenID, revokedBy, reason string) error
	TouchAgentToken(ctx context.Context, tokenID string) error

	// Legacy compatibility shims used by older call sites.
	GetTokenByHash(ctx context.Context, hash string) (*controlmodel.AgentToken, error)
	GetToken(ctx context.Context, id string) (*controlmodel.AgentToken, error)
	ListTokens(ctx context.Context, agentID string, page Page) (PageResult[*controlmodel.AgentToken], error)
	RevokeToken(ctx context.Context, tokenID, revokedBy, reason string) error
	TouchToken(ctx context.Context, tokenID string) error
}

// CredentialStore owns outbound connector secret persistence.
// Supports all four tiers: external reference, encrypted local, ephemeral, pass-through.
type CredentialStore interface {
	// CRUD
	GetCredential(ctx context.Context, id string) (*controlmodel.ConnectorCredential, error)
	StoreCredential(ctx context.Context, c *controlmodel.ConnectorCredential) error
	DeleteCredential(ctx context.Context, id string) error
	ListCredentials(ctx context.Context, envID string, page Page) (PageResult[*controlmodel.ConnectorCredential], error)

	// Lookup: policy-driven resolution (not singleton by connector).
	// Fallback chain: exact label match → "primary" label → any active credential for this connector.
	// Returns ErrNoCredential if nothing found — caller decides whether to fall through to passthrough.
	ResolveCredential(ctx context.Context, filter *controlmodel.CredentialFilter) (*controlmodel.ConnectorCredential, error)

	// Lifecycle mutations
	RotateCredential(ctx context.Context, id string, newBlob []byte, wrappingKeyID, rotatedBy string) error
	RevokeCredential(ctx context.Context, id string, revokedBy, reason string) error
	TouchCredential(ctx context.Context, id string) error // updates last_used_at; async-safe
}

// RoleStore owns RBAC roles.
type RoleStore interface {
	InsertRole(ctx context.Context, role *controlmodel.Role) error
	GetRole(ctx context.Context, id string) (*controlmodel.Role, error)
	ListRoles(ctx context.Context, orgID string, page Page) (PageResult[*controlmodel.Role], error)
	UpdateRole(ctx context.Context, id string, role *controlmodel.Role) error
	DeleteRole(ctx context.Context, id string) error
}

// BindingStore owns RBAC bindings.
type BindingStore interface {
	InsertBinding(ctx context.Context, binding *controlmodel.Binding) error
	GetBinding(ctx context.Context, id string) (*controlmodel.Binding, error)
	ListBindings(ctx context.Context, orgID string, page Page) (PageResult[*controlmodel.Binding], error)
	DeleteBinding(ctx context.Context, id string) error
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
	BudgetStore
	RunStore
	ActionStore
	CostStore
	TokenStore
	CredentialStore
	RoleStore
	BindingStore
	StoreLifecycle
}

// SpanStore holds trace spans. Separated from AppStore because it uses a
// different backend optimized for append-heavy analytical queries (DuckDB by default).
type SpanStore interface {
	OpenSpan(ctx context.Context, span *runtimemodel.SpanRow) error
	CloseSpan(ctx context.Context, spanID string, end *runtimemodel.SpanEnd) error
	GetSpan(ctx context.Context, spanID string) (*runtimemodel.SpanRow, error)
	QuerySpans(ctx context.Context, filter *runtimemodel.SpanFilter, page Page) (PageResult[*runtimemodel.SpanRow], error)
	SpendByDimension(ctx context.Context, groupBy string, filter *runtimemodel.SpanFilter) (map[string]money.Amount, error)
	Migrate(ctx context.Context) error
	Close() error
}

// AuditStore holds append-only audit logs.
// Separated from AppStore: different backend may be preferred for compliance, and the
// write-only append pattern is distinct from the mutable control-plane data.
type AuditStore interface {
	AppendAudit(ctx context.Context, entry *auditmodel.AuditLog) error
	QueryAudits(ctx context.Context, filter *auditmodel.AuditFilter, page Page) (PageResult[*auditmodel.AuditLog], error)
	Migrate(ctx context.Context) error
	Close() error
}
