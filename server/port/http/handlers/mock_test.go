package handlers

import (
	"context"

	"github.com/kave-io/kave/core/store"
)

// mockAppStore is a minimal in-memory stub for store.AppStore used in tests.
type mockAppStore struct {
	createAgent    func(context.Context, *store.Agent) error
	getAgentByID   func(context.Context, string) (*store.Agent, error)
	getAgentByName func(context.Context, string, string) (*store.Agent, error)
	updateAgent    func(context.Context, string, *store.AgentUpdate) error
	listAgents     func(context.Context, string) ([]*store.Agent, error)
	createPolicy   func(context.Context, *store.Policy) error
	getPolicy      func(context.Context, string) (*store.Policy, error)
	listRuns       func(context.Context, *store.RunFilter) ([]*store.Run, error)
	getRunByID     func(context.Context, string) (*store.Run, error)
	getSpendReport func(context.Context, *store.SpendFilter) (*store.SpendReport, error)
	getPriceBook   func(context.Context) (*store.PriceBook, error)
	savePriceBook  func(context.Context, *store.PriceBook) error
}

func (m *mockAppStore) CreateAgent(ctx context.Context, a *store.Agent) error {
	if m.createAgent != nil {
		return m.createAgent(ctx, a)
	}
	return nil
}
func (m *mockAppStore) CreateWorkspace(ctx context.Context, w *store.Workspace) error { return nil }
func (m *mockAppStore) GetWorkspace(ctx context.Context, id string) (*store.Workspace, error) {
	return nil, nil
}
func (m *mockAppStore) GetAgentByID(ctx context.Context, id string) (*store.Agent, error) {
	if m.getAgentByID != nil {
		return m.getAgentByID(ctx, id)
	}
	return nil, nil
}
func (m *mockAppStore) GetAgentByName(ctx context.Context, wsID, name string) (*store.Agent, error) {
	if m.getAgentByName != nil {
		return m.getAgentByName(ctx, wsID, name)
	}
	return nil, nil
}
func (m *mockAppStore) UpdateAgent(ctx context.Context, id string, u *store.AgentUpdate) error {
	if m.updateAgent != nil {
		return m.updateAgent(ctx, id, u)
	}
	return nil
}
func (m *mockAppStore) ListAgents(ctx context.Context, wsID string) ([]*store.Agent, error) {
	if m.listAgents != nil {
		return m.listAgents(ctx, wsID)
	}
	return nil, nil
}
func (m *mockAppStore) CreatePolicy(ctx context.Context, p *store.Policy) error {
	if m.createPolicy != nil {
		return m.createPolicy(ctx, p)
	}
	return nil
}
func (m *mockAppStore) GetPolicy(ctx context.Context, id string) (*store.Policy, error) {
	if m.getPolicy != nil {
		return m.getPolicy(ctx, id)
	}
	return nil, nil
}
func (m *mockAppStore) GetAgentPolicy(ctx context.Context, agentID string) (*store.Policy, error) {
	return nil, nil
}
func (m *mockAppStore) CreateRun(ctx context.Context, r *store.Run) error { return nil }
func (m *mockAppStore) CreateAction(ctx context.Context, a *store.ActionRecord) error {
	return nil
}
func (m *mockAppStore) GetRunByID(ctx context.Context, id string) (*store.Run, error) {
	if m.getRunByID != nil {
		return m.getRunByID(ctx, id)
	}
	return nil, nil
}
func (m *mockAppStore) UpdateRun(ctx context.Context, id string, u *store.RunUpdate) error {
	return nil
}
func (m *mockAppStore) ListRuns(ctx context.Context, f *store.RunFilter) ([]*store.Run, error) {
	if m.listRuns != nil {
		return m.listRuns(ctx, f)
	}
	return nil, nil
}
func (m *mockAppStore) InsertBudgetEntry(ctx context.Context, e *store.BudgetEntry) error {
	return nil
}
func (m *mockAppStore) AddRunSpend(ctx context.Context, runID string, cost float64) error {
	return nil
}
func (m *mockAppStore) SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (float64, error) {
	return 0, nil
}
func (m *mockAppStore) GetSpendReport(ctx context.Context, f *store.SpendFilter) (*store.SpendReport, error) {
	if m.getSpendReport != nil {
		return m.getSpendReport(ctx, f)
	}
	return &store.SpendReport{ByAgent: map[string]float64{}, ByConnector: map[string]float64{}, ByModel: map[string]float64{}}, nil
}
func (m *mockAppStore) GetPriceBook(ctx context.Context) (*store.PriceBook, error) {
	if m.getPriceBook != nil {
		return m.getPriceBook(ctx)
	}
	return nil, nil
}
func (m *mockAppStore) SavePriceBook(ctx context.Context, book *store.PriceBook) error {
	if m.savePriceBook != nil {
		return m.savePriceBook(ctx, book)
	}
	return nil
}
func (m *mockAppStore) InsertAgentToken(ctx context.Context, t *store.AgentToken) error { return nil }
func (m *mockAppStore) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	return false, nil
}
func (m *mockAppStore) InsertRevokedToken(ctx context.Context, tokenID string) error { return nil }
func (m *mockAppStore) GetCredential(ctx context.Context, wsID, connector string) (*store.Credential, error) {
	return nil, nil
}
func (m *mockAppStore) StoreCredential(ctx context.Context, c *store.Credential) error { return nil }
func (m *mockAppStore) DeleteCredential(ctx context.Context, id string) error          { return nil }
func (m *mockAppStore) WithTx(ctx context.Context, fn func(store.AppStore) error) error {
	return fn(m)
}
func (m *mockAppStore) Migrate(ctx context.Context) error { return nil }
func (m *mockAppStore) Close() error                      { return nil }

// mockSpanStore is a minimal stub for store.SpanStore used in tests.
type mockSpanStore struct {
	querySpans func(context.Context, *store.SpanFilter) ([]*store.SpanRow, error)
}

func (m *mockSpanStore) WriteSpan(ctx context.Context, s *store.SpanRow) error  { return nil }
func (m *mockSpanStore) UpdateSpan(ctx context.Context, s *store.SpanRow) error { return nil }
func (m *mockSpanStore) GetSpan(ctx context.Context, id string) (*store.SpanRow, error) {
	return nil, nil
}
func (m *mockSpanStore) QuerySpans(ctx context.Context, f *store.SpanFilter) ([]*store.SpanRow, error) {
	if m.querySpans != nil {
		return m.querySpans(ctx, f)
	}
	return nil, nil
}
func (m *mockSpanStore) SpendByDimension(ctx context.Context, groupBy string, f *store.SpanFilter) (map[string]float64, error) {
	return nil, nil
}
func (m *mockSpanStore) Migrate(ctx context.Context) error { return nil }
func (m *mockSpanStore) Close() error                      { return nil }

// helpers
func strPtr(s string) *string   { return &s }
func f64Ptr(f float64) *float64 { return &f }
