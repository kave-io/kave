// Package fake provides no-op store implementations for tests.
package fake

import (
	"context"

	auditmodel "github.com/kave-io/kave/core/model/audit"
	controlmodel "github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
)

// Store is a no-op implementation of store.AppStore, store.SpanStore, and
// store.AuditStore. Tests can use the composed feature fakes below directly
// or embed the whole Store when they need the full interface set.
type Store struct {
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
	SpanStore
	AuditStore
}

type OrgStore struct{}

type UserStore struct{}

type MembershipStore struct{}

type ProjectStore struct{}

type EnvironmentStore struct{}

type AgentStore struct{}

type PolicyStore struct{}

type BudgetStore struct{}

type RunStore struct{}

type ActionStore struct{}

type CostStore struct{}

type TokenStore struct{}

type CredentialStore struct{}

type RoleStore struct{}

type BindingStore struct{}

type SpanStore struct{}

type AuditStore struct{}

func emptyPageResult[T any]() store.PageResult[T] { return store.PageResult[T]{} }

// OrgStore
func (OrgStore) CreateOrg(context.Context, *controlmodel.Organization) error { return nil }
func (OrgStore) GetOrg(context.Context, string) (*controlmodel.Organization, error) {
	return nil, store.ErrNotFound
}
func (OrgStore) GetOrgBySlug(context.Context, string) (*controlmodel.Organization, error) {
	return nil, store.ErrNotFound
}
func (OrgStore) ListOrgs(context.Context, store.Page) (store.PageResult[*controlmodel.Organization], error) {
	return emptyPageResult[*controlmodel.Organization](), nil
}

// UserStore
func (UserStore) CreateUser(context.Context, *controlmodel.User) error { return nil }
func (UserStore) GetUser(context.Context, string) (*controlmodel.User, error) {
	return nil, store.ErrNotFound
}
func (UserStore) GetUserByEmail(context.Context, string, string) (*controlmodel.User, error) {
	return nil, store.ErrNotFound
}
func (UserStore) UpdateUser(context.Context, string, *controlmodel.UserUpdate) error { return nil }

// MembershipStore
func (MembershipStore) AddMember(context.Context, *controlmodel.Membership) error { return nil }
func (MembershipStore) GetMembership(context.Context, string, string) (*controlmodel.Membership, error) {
	return nil, store.ErrNotFound
}
func (MembershipStore) ListMembers(context.Context, string, store.Page) (store.PageResult[*controlmodel.Membership], error) {
	return emptyPageResult[*controlmodel.Membership](), nil
}
func (MembershipStore) RemoveMember(context.Context, string, string) error { return nil }

// ProjectStore
func (ProjectStore) CreateProject(context.Context, *controlmodel.Project) error { return nil }
func (ProjectStore) GetProject(context.Context, string) (*controlmodel.Project, error) {
	return nil, store.ErrNotFound
}
func (ProjectStore) ListProjects(context.Context, string, store.Page) (store.PageResult[*controlmodel.Project], error) {
	return emptyPageResult[*controlmodel.Project](), nil
}

// EnvironmentStore
func (EnvironmentStore) CreateEnvironment(context.Context, *controlmodel.Environment) error {
	return nil
}
func (EnvironmentStore) GetEnvironment(context.Context, string) (*controlmodel.Environment, error) {
	return nil, store.ErrNotFound
}
func (EnvironmentStore) GetEnvironmentBySlug(context.Context, string, string) (*controlmodel.Environment, error) {
	return nil, store.ErrNotFound
}
func (EnvironmentStore) ListEnvironments(context.Context, string, store.Page) (store.PageResult[*controlmodel.Environment], error) {
	return emptyPageResult[*controlmodel.Environment](), nil
}

// AgentStore
func (AgentStore) CreateAgent(context.Context, *controlmodel.Agent) error { return nil }
func (AgentStore) GetAgentByID(context.Context, string) (*controlmodel.Agent, error) {
	return nil, store.ErrNotFound
}
func (AgentStore) GetAgentByName(context.Context, string, string) (*controlmodel.Agent, error) {
	return nil, store.ErrNotFound
}
func (AgentStore) UpdateAgent(context.Context, string, *controlmodel.AgentUpdate) error { return nil }
func (AgentStore) ListAgents(context.Context, string, store.Page) (store.PageResult[*controlmodel.Agent], error) {
	return emptyPageResult[*controlmodel.Agent](), nil
}
func (AgentStore) DeleteAgent(context.Context, string, string) error  { return nil }
func (AgentStore) RestoreAgent(context.Context, string, string) error { return nil }

// PolicyStore
func (PolicyStore) CreatePolicy(context.Context, *controlmodel.PolicyRecord) error { return nil }
func (PolicyStore) GetPolicy(context.Context, string) (*controlmodel.PolicyRecord, error) {
	return nil, store.ErrNotFound
}
func (PolicyStore) UpdatePolicy(context.Context, string, *controlmodel.PolicyUpdate) error {
	return nil
}
func (PolicyStore) DeletePolicy(context.Context, string) error { return nil }
func (PolicyStore) GetAgentPolicy(context.Context, string) (*controlmodel.PolicyRecord, error) {
	return nil, store.ErrNotFound
}
func (PolicyStore) ListPolicies(context.Context, string, store.Page) (store.PageResult[*controlmodel.PolicyRecord], error) {
	return emptyPageResult[*controlmodel.PolicyRecord](), nil
}

// BudgetStore
func (BudgetStore) CreateBudget(context.Context, *controlmodel.Budget) error { return nil }
func (BudgetStore) GetBudget(context.Context, string) (*controlmodel.Budget, error) {
	return nil, store.ErrNotFound
}
func (BudgetStore) DeleteBudget(context.Context, string) error { return nil }

// RunStore
func (RunStore) CreateRun(context.Context, *runtimemodel.RunRecord) error { return nil }
func (RunStore) GetRunByID(context.Context, string) (*runtimemodel.RunRecord, error) {
	return nil, store.ErrNotFound
}
func (RunStore) GetRunByIdempotencyKey(context.Context, string, string) (*runtimemodel.RunRecord, error) {
	return nil, store.ErrNotFound
}
func (RunStore) UpdateRun(context.Context, string, *runtimemodel.RunUpdate) error { return nil }
func (RunStore) ListRuns(context.Context, *runtimemodel.RunFilter, store.Page) (store.PageResult[*runtimemodel.RunRecord], error) {
	return emptyPageResult[*runtimemodel.RunRecord](), nil
}

// ActionStore
func (ActionStore) CreateAction(context.Context, *runtimemodel.ActionRecord) error { return nil }
func (ActionStore) GetAction(context.Context, string) (*runtimemodel.ActionRecord, error) {
	return nil, store.ErrNotFound
}
func (ActionStore) ListActionsByRun(context.Context, string, store.Page) (store.PageResult[*runtimemodel.ActionRecord], error) {
	return emptyPageResult[*runtimemodel.ActionRecord](), nil
}

// CostStore
func (CostStore) GetPriceBook(context.Context) (*runtimemodel.PriceBook, error) {
	return nil, store.ErrNotFound
}
func (CostStore) SavePriceBook(context.Context, *runtimemodel.PriceBook) error { return nil }
func (CostStore) ListFXRates(context.Context) ([]runtimemodel.FXRateRecord, error) {
	return nil, nil
}
func (CostStore) GetFXRate(context.Context, money.CurrencyCode, money.CurrencyCode) (*runtimemodel.FXRateRecord, error) {
	return nil, store.ErrNotFound
}
func (CostStore) UpsertFXRates(context.Context, []runtimemodel.FXRateRecord) error { return nil }
func (CostStore) ListFXCurrencies(context.Context) ([]runtimemodel.FXCurrencyRecord, error) {
	return nil, nil
}
func (CostStore) UpsertFXCurrencies(context.Context, []runtimemodel.FXCurrencyRecord) error {
	return nil
}
func (CostStore) InsertBudgetEntry(context.Context, *runtimemodel.BudgetEntry) error { return nil }
func (CostStore) AddRunSpend(context.Context, string, money.Amount) error            { return nil }
func (CostStore) SumAgentSpend(context.Context, string, int64) (money.Amount, error) { return 0, nil }
func (CostStore) GetSpendReport(context.Context, *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	return nil, store.ErrNotFound
}

// TokenStore
func (TokenStore) InsertSession(context.Context, *controlmodel.Session) error { return nil }
func (TokenStore) GetSessionByHash(context.Context, string) (*controlmodel.Session, error) {
	return nil, store.ErrNotFound
}
func (TokenStore) GetSession(context.Context, string) (*controlmodel.Session, error) {
	return nil, store.ErrNotFound
}
func (TokenStore) ListSessions(context.Context, string, store.Page) (store.PageResult[*controlmodel.Session], error) {
	return emptyPageResult[*controlmodel.Session](), nil
}
func (TokenStore) RevokeSession(context.Context, string, string) error { return nil }
func (TokenStore) TouchSession(context.Context, string) error          { return nil }

func (TokenStore) InsertAPIToken(context.Context, *controlmodel.APIToken) error { return nil }
func (TokenStore) GetAPITokenByHash(context.Context, string) (*controlmodel.APIToken, error) {
	return nil, store.ErrNotFound
}
func (TokenStore) GetAPIToken(context.Context, string) (*controlmodel.APIToken, error) {
	return nil, store.ErrNotFound
}
func (TokenStore) ListAPITokens(context.Context, string, store.Page) (store.PageResult[*controlmodel.APIToken], error) {
	return emptyPageResult[*controlmodel.APIToken](), nil
}
func (TokenStore) RevokeAPIToken(context.Context, string, string, string) error { return nil }
func (TokenStore) TouchAPIToken(context.Context, string) error                  { return nil }

func (TokenStore) InsertAgentToken(context.Context, *controlmodel.AgentToken) error { return nil }
func (TokenStore) GetAgentTokenByHash(context.Context, string) (*controlmodel.AgentToken, error) {
	return nil, store.ErrNotFound
}
func (TokenStore) GetAgentToken(context.Context, string) (*controlmodel.AgentToken, error) {
	return nil, store.ErrNotFound
}
func (TokenStore) ListAgentTokens(context.Context, string, store.Page) (store.PageResult[*controlmodel.AgentToken], error) {
	return emptyPageResult[*controlmodel.AgentToken](), nil
}
func (TokenStore) RevokeAgentToken(context.Context, string, string, string) error { return nil }
func (TokenStore) TouchAgentToken(context.Context, string) error                  { return nil }

func (TokenStore) GetTokenByHash(ctx context.Context, hash string) (*controlmodel.AgentToken, error) {
	return TokenStore{}.GetAgentTokenByHash(ctx, hash)
}
func (TokenStore) GetToken(ctx context.Context, id string) (*controlmodel.AgentToken, error) {
	return TokenStore{}.GetAgentToken(ctx, id)
}
func (TokenStore) ListTokens(ctx context.Context, agentID string, page store.Page) (store.PageResult[*controlmodel.AgentToken], error) {
	return TokenStore{}.ListAgentTokens(ctx, agentID, page)
}
func (TokenStore) RevokeToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	return TokenStore{}.RevokeAgentToken(ctx, tokenID, revokedBy, reason)
}
func (TokenStore) TouchToken(ctx context.Context, tokenID string) error {
	return TokenStore{}.TouchAgentToken(ctx, tokenID)
}

// CredentialStore
func (CredentialStore) GetCredential(context.Context, string) (*controlmodel.ConnectorCredential, error) {
	return nil, store.ErrNotFound
}
func (CredentialStore) StoreCredential(context.Context, *controlmodel.ConnectorCredential) error {
	return nil
}
func (CredentialStore) DeleteCredential(context.Context, string) error { return nil }
func (CredentialStore) ListCredentials(context.Context, string, store.Page) (store.PageResult[*controlmodel.ConnectorCredential], error) {
	return emptyPageResult[*controlmodel.ConnectorCredential](), nil
}
func (CredentialStore) ResolveCredential(context.Context, *controlmodel.CredentialFilter) (*controlmodel.ConnectorCredential, error) {
	return nil, store.ErrNoCredential
}
func (CredentialStore) RotateCredential(context.Context, string, []byte, string, string) error {
	return nil
}
func (CredentialStore) RevokeCredential(context.Context, string, string, string) error { return nil }
func (CredentialStore) TouchCredential(context.Context, string) error                  { return nil }

// RoleStore
func (RoleStore) InsertRole(context.Context, *controlmodel.Role) error { return nil }
func (RoleStore) GetRole(context.Context, string) (*controlmodel.Role, error) {
	return nil, store.ErrNotFound
}
func (RoleStore) ListRoles(context.Context, string, store.Page) (store.PageResult[*controlmodel.Role], error) {
	return emptyPageResult[*controlmodel.Role](), nil
}
func (RoleStore) UpdateRole(context.Context, string, *controlmodel.Role) error { return nil }
func (RoleStore) DeleteRole(context.Context, string) error                     { return nil }

// BindingStore
func (BindingStore) InsertBinding(context.Context, *controlmodel.Binding) error { return nil }
func (BindingStore) GetBinding(context.Context, string) (*controlmodel.Binding, error) {
	return nil, store.ErrNotFound
}
func (BindingStore) ListBindings(context.Context, string, store.Page) (store.PageResult[*controlmodel.Binding], error) {
	return emptyPageResult[*controlmodel.Binding](), nil
}
func (BindingStore) DeleteBinding(context.Context, string) error { return nil }

// SpanStore
func (SpanStore) OpenSpan(context.Context, *runtimemodel.SpanRow) error { return nil }
func (SpanStore) CloseSpan(context.Context, string, *runtimemodel.SpanEnd) error {
	return nil
}
func (SpanStore) GetSpan(context.Context, string) (*runtimemodel.SpanRow, error) {
	return nil, store.ErrNotFound
}
func (SpanStore) QuerySpans(context.Context, *runtimemodel.SpanFilter, store.Page) (store.PageResult[*runtimemodel.SpanRow], error) {
	return emptyPageResult[*runtimemodel.SpanRow](), nil
}
func (SpanStore) SpendByDimension(context.Context, string, *runtimemodel.SpanFilter) (map[string]money.Amount, error) {
	return map[string]money.Amount{}, nil
}
func (SpanStore) Migrate(context.Context) error { return nil }
func (SpanStore) Close() error                  { return nil }

// AuditStore
func (AuditStore) AppendAudit(context.Context, *auditmodel.AuditLog) error { return nil }
func (AuditStore) QueryAudits(context.Context, *auditmodel.AuditFilter, store.Page) (store.PageResult[*auditmodel.AuditLog], error) {
	return emptyPageResult[*auditmodel.AuditLog](), nil
}
func (AuditStore) Migrate(context.Context) error { return nil }
func (AuditStore) Close() error                  { return nil }

func (Store) WithTx(ctx context.Context, fn func(store.AppStore) error) error {
	if fn == nil {
		return nil
	}
	return fn(Store{})
}

func (Store) Migrate(context.Context) error { return nil }

func (Store) Close() error { return nil }
