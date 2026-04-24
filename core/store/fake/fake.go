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
// store.AuditStore. Tests can use the type directly or via the aliases below.
type Store struct{}

type (
	AppStore         = Store
	OrgStore         = Store
	UserStore        = Store
	MembershipStore  = Store
	ProjectStore     = Store
	EnvironmentStore = Store
	AgentStore       = Store
	PolicyStore      = Store
	BudgetStore      = Store
	RunStore         = Store
	ActionStore      = Store
	CostStore        = Store
	TokenStore       = Store
	CredentialStore  = Store
	RoleStore        = Store
	BindingStore     = Store
	SpanStore        = Store
	AuditStore       = Store
)

func (Store) CreateOrg(context.Context, *controlmodel.Organization) error { return nil }
func (Store) GetOrg(context.Context, string) (*controlmodel.Organization, error) {
	return nil, store.ErrNotFound
}
func (Store) GetOrgBySlug(context.Context, string) (*controlmodel.Organization, error) {
	return nil, store.ErrNotFound
}
func (Store) ListOrgs(context.Context, store.Page) (store.PageResult[*controlmodel.Organization], error) {
	return store.PageResult[*controlmodel.Organization]{}, nil
}

func (Store) CreateUser(context.Context, *controlmodel.User) error { return nil }
func (Store) GetUser(context.Context, string) (*controlmodel.User, error) {
	return nil, store.ErrNotFound
}
func (Store) GetUserByEmail(context.Context, string, string) (*controlmodel.User, error) {
	return nil, store.ErrNotFound
}
func (Store) UpdateUser(context.Context, string, *controlmodel.UserUpdate) error { return nil }

func (Store) AddMember(context.Context, *controlmodel.Membership) error { return nil }
func (Store) GetMembership(context.Context, string, string) (*controlmodel.Membership, error) {
	return nil, store.ErrNotFound
}
func (Store) ListMembers(context.Context, string, store.Page) (store.PageResult[*controlmodel.Membership], error) {
	return store.PageResult[*controlmodel.Membership]{}, nil
}
func (Store) RemoveMember(context.Context, string, string) error { return nil }

func (Store) CreateProject(context.Context, *controlmodel.Project) error { return nil }
func (Store) GetProject(context.Context, string) (*controlmodel.Project, error) {
	return nil, store.ErrNotFound
}
func (Store) ListProjects(context.Context, string, store.Page) (store.PageResult[*controlmodel.Project], error) {
	return store.PageResult[*controlmodel.Project]{}, nil
}

func (Store) CreateEnvironment(context.Context, *controlmodel.Environment) error { return nil }
func (Store) GetEnvironment(context.Context, string) (*controlmodel.Environment, error) {
	return nil, store.ErrNotFound
}
func (Store) GetEnvironmentBySlug(context.Context, string, string) (*controlmodel.Environment, error) {
	return nil, store.ErrNotFound
}
func (Store) ListEnvironments(context.Context, string, store.Page) (store.PageResult[*controlmodel.Environment], error) {
	return store.PageResult[*controlmodel.Environment]{}, nil
}

func (Store) CreateAgent(context.Context, *controlmodel.Agent) error { return nil }
func (Store) GetAgentByID(context.Context, string) (*controlmodel.Agent, error) {
	return nil, store.ErrNotFound
}
func (Store) GetAgentByName(context.Context, string, string) (*controlmodel.Agent, error) {
	return nil, store.ErrNotFound
}
func (Store) UpdateAgent(context.Context, string, *controlmodel.AgentUpdate) error { return nil }
func (Store) ListAgents(context.Context, string, store.Page) (store.PageResult[*controlmodel.Agent], error) {
	return store.PageResult[*controlmodel.Agent]{}, nil
}
func (Store) DeleteAgent(context.Context, string, string) error  { return nil }
func (Store) RestoreAgent(context.Context, string, string) error { return nil }

func (Store) CreatePolicy(context.Context, *controlmodel.PolicyRecord) error { return nil }
func (Store) GetPolicy(context.Context, string) (*controlmodel.PolicyRecord, error) {
	return nil, store.ErrNotFound
}
func (Store) UpdatePolicy(context.Context, string, *controlmodel.PolicyUpdate) error { return nil }
func (Store) DeletePolicy(context.Context, string) error                             { return nil }
func (Store) GetAgentPolicy(context.Context, string) (*controlmodel.PolicyRecord, error) {
	return nil, store.ErrNotFound
}
func (Store) ListPolicies(context.Context, string, store.Page) (store.PageResult[*controlmodel.PolicyRecord], error) {
	return store.PageResult[*controlmodel.PolicyRecord]{}, nil
}

func (Store) CreateBudget(context.Context, *controlmodel.Budget) error { return nil }
func (Store) GetBudget(context.Context, string) (*controlmodel.Budget, error) {
	return nil, store.ErrNotFound
}
func (Store) DeleteBudget(context.Context, string) error { return nil }

func (Store) CreateRun(context.Context, *runtimemodel.RunRecord) error { return nil }
func (Store) GetRunByID(context.Context, string) (*runtimemodel.RunRecord, error) {
	return nil, store.ErrNotFound
}
func (Store) GetRunByIdempotencyKey(context.Context, string, string) (*runtimemodel.RunRecord, error) {
	return nil, store.ErrNotFound
}
func (Store) UpdateRun(context.Context, string, *runtimemodel.RunUpdate) error { return nil }
func (Store) ListRuns(context.Context, *runtimemodel.RunFilter, store.Page) (store.PageResult[*runtimemodel.RunRecord], error) {
	return store.PageResult[*runtimemodel.RunRecord]{}, nil
}

func (Store) CreateAction(context.Context, *runtimemodel.ActionRecord) error { return nil }
func (Store) GetAction(context.Context, string) (*runtimemodel.ActionRecord, error) {
	return nil, store.ErrNotFound
}
func (Store) ListActionsByRun(context.Context, string, store.Page) (store.PageResult[*runtimemodel.ActionRecord], error) {
	return store.PageResult[*runtimemodel.ActionRecord]{}, nil
}

func (Store) GetPriceBook(context.Context) (*runtimemodel.PriceBook, error) {
	return nil, store.ErrNotFound
}
func (Store) SavePriceBook(context.Context, *runtimemodel.PriceBook) error { return nil }
func (Store) ListFXRates(context.Context) ([]runtimemodel.FXRateRecord, error) {
	return nil, nil
}
func (Store) GetFXRate(context.Context, money.CurrencyCode, money.CurrencyCode) (*runtimemodel.FXRateRecord, error) {
	return nil, store.ErrNotFound
}
func (Store) UpsertFXRates(context.Context, []runtimemodel.FXRateRecord) error { return nil }
func (Store) ListFXCurrencies(context.Context) ([]runtimemodel.FXCurrencyRecord, error) {
	return nil, nil
}
func (Store) UpsertFXCurrencies(context.Context, []runtimemodel.FXCurrencyRecord) error { return nil }
func (Store) InsertBudgetEntry(context.Context, *runtimemodel.BudgetEntry) error        { return nil }
func (Store) AddRunSpend(context.Context, string, money.Amount) error                   { return nil }
func (Store) SumAgentSpend(context.Context, string, int64) (money.Amount, error)        { return 0, nil }
func (Store) GetSpendReport(context.Context, *runtimemodel.SpendFilter) (*runtimemodel.SpendReport, error) {
	return nil, store.ErrNotFound
}

func (Store) InsertSession(context.Context, *controlmodel.Session) error { return nil }
func (Store) GetSessionByHash(context.Context, string) (*controlmodel.Session, error) {
	return nil, store.ErrNotFound
}
func (Store) GetSession(context.Context, string) (*controlmodel.Session, error) {
	return nil, store.ErrNotFound
}
func (Store) ListSessions(context.Context, string, store.Page) (store.PageResult[*controlmodel.Session], error) {
	return store.PageResult[*controlmodel.Session]{}, nil
}
func (Store) RevokeSession(context.Context, string, string) error { return nil }
func (Store) TouchSession(context.Context, string) error          { return nil }

func (Store) InsertAPIToken(context.Context, *controlmodel.APIToken) error { return nil }
func (Store) GetAPITokenByHash(context.Context, string) (*controlmodel.APIToken, error) {
	return nil, store.ErrNotFound
}
func (Store) GetAPIToken(context.Context, string) (*controlmodel.APIToken, error) {
	return nil, store.ErrNotFound
}
func (Store) ListAPITokens(context.Context, string, store.Page) (store.PageResult[*controlmodel.APIToken], error) {
	return store.PageResult[*controlmodel.APIToken]{}, nil
}
func (Store) RevokeAPIToken(context.Context, string, string, string) error { return nil }
func (Store) TouchAPIToken(context.Context, string) error                  { return nil }

func (Store) InsertAgentToken(context.Context, *controlmodel.AgentToken) error { return nil }
func (Store) GetAgentTokenByHash(context.Context, string) (*controlmodel.AgentToken, error) {
	return nil, store.ErrNotFound
}
func (Store) GetAgentToken(context.Context, string) (*controlmodel.AgentToken, error) {
	return nil, store.ErrNotFound
}
func (Store) ListAgentTokens(context.Context, string, store.Page) (store.PageResult[*controlmodel.AgentToken], error) {
	return store.PageResult[*controlmodel.AgentToken]{}, nil
}
func (Store) RevokeAgentToken(context.Context, string, string, string) error { return nil }
func (Store) TouchAgentToken(context.Context, string) error                  { return nil }

func (Store) GetTokenByHash(ctx context.Context, hash string) (*controlmodel.AgentToken, error) {
	return Store{}.GetAgentTokenByHash(ctx, hash)
}
func (Store) GetToken(ctx context.Context, id string) (*controlmodel.AgentToken, error) {
	return Store{}.GetAgentToken(ctx, id)
}
func (Store) ListTokens(ctx context.Context, agentID string, page store.Page) (store.PageResult[*controlmodel.AgentToken], error) {
	return Store{}.ListAgentTokens(ctx, agentID, page)
}
func (Store) RevokeToken(ctx context.Context, tokenID, revokedBy, reason string) error {
	return Store{}.RevokeAgentToken(ctx, tokenID, revokedBy, reason)
}
func (Store) TouchToken(ctx context.Context, tokenID string) error {
	return Store{}.TouchAgentToken(ctx, tokenID)
}

func (Store) GetCredential(context.Context, string) (*controlmodel.ConnectorCredential, error) {
	return nil, store.ErrNotFound
}
func (Store) StoreCredential(context.Context, *controlmodel.ConnectorCredential) error { return nil }
func (Store) DeleteCredential(context.Context, string) error                           { return nil }
func (Store) ListCredentials(context.Context, string, store.Page) (store.PageResult[*controlmodel.ConnectorCredential], error) {
	return store.PageResult[*controlmodel.ConnectorCredential]{}, nil
}
func (Store) ResolveCredential(context.Context, *controlmodel.CredentialFilter) (*controlmodel.ConnectorCredential, error) {
	return nil, store.ErrNoCredential
}
func (Store) RotateCredential(context.Context, string, []byte, string, string) error { return nil }
func (Store) RevokeCredential(context.Context, string, string, string) error         { return nil }
func (Store) TouchCredential(context.Context, string) error                          { return nil }

func (Store) InsertRole(context.Context, *controlmodel.Role) error { return nil }
func (Store) GetRole(context.Context, string) (*controlmodel.Role, error) {
	return nil, store.ErrNotFound
}
func (Store) ListRoles(context.Context, string, store.Page) (store.PageResult[*controlmodel.Role], error) {
	return store.PageResult[*controlmodel.Role]{}, nil
}
func (Store) UpdateRole(context.Context, string, *controlmodel.Role) error { return nil }
func (Store) DeleteRole(context.Context, string) error                     { return nil }

func (Store) InsertBinding(context.Context, *controlmodel.Binding) error { return nil }
func (Store) GetBinding(context.Context, string) (*controlmodel.Binding, error) {
	return nil, store.ErrNotFound
}
func (Store) ListBindings(context.Context, string, store.Page) (store.PageResult[*controlmodel.Binding], error) {
	return store.PageResult[*controlmodel.Binding]{}, nil
}
func (Store) DeleteBinding(context.Context, string) error { return nil }

func (Store) WithTx(ctx context.Context, fn func(store.AppStore) error) error {
	if fn == nil {
		return nil
	}
	return fn(Store{})
}
func (Store) Migrate(context.Context) error { return nil }
func (Store) Close() error                  { return nil }

func (Store) OpenSpan(context.Context, *runtimemodel.SpanRow) error { return nil }
func (Store) CloseSpan(context.Context, string, *runtimemodel.SpanEnd) error {
	return nil
}
func (Store) GetSpan(context.Context, string) (*runtimemodel.SpanRow, error) {
	return nil, store.ErrNotFound
}
func (Store) QuerySpans(context.Context, *runtimemodel.SpanFilter, store.Page) (store.PageResult[*runtimemodel.SpanRow], error) {
	return store.PageResult[*runtimemodel.SpanRow]{}, nil
}
func (Store) SpendByDimension(context.Context, string, *runtimemodel.SpanFilter) (map[string]money.Amount, error) {
	return map[string]money.Amount{}, nil
}

func (Store) AppendAudit(context.Context, *auditmodel.AuditLog) error { return nil }
func (Store) QueryAudits(context.Context, *auditmodel.AuditFilter, store.Page) (store.PageResult[*auditmodel.AuditLog], error) {
	return store.PageResult[*auditmodel.AuditLog]{}, nil
}
