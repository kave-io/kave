// Package store defines storage interfaces and data types for the Kave control plane.
// These interfaces are implemented by server/store/* packages using SQLite, Postgres, and DuckDB.
package store

import "context"

// AppStore is the primary application data store.
// It holds workspaces, agents, policies, runs, budget ledger, tokens, and credentials.
type AppStore interface {
	// Workspace
	CreateWorkspace(ctx context.Context, w *Workspace) error
	GetWorkspace(ctx context.Context, id string) (*Workspace, error)

	// Agent
	CreateAgent(ctx context.Context, a *Agent) error
	GetAgentByID(ctx context.Context, id string) (*Agent, error)
	GetAgentByName(ctx context.Context, workspaceID, name string) (*Agent, error)
	UpdateAgent(ctx context.Context, id string, update *AgentUpdate) error
	ListAgents(ctx context.Context, workspaceID string) ([]*Agent, error)

	// Policy
	CreatePolicy(ctx context.Context, p *Policy) error
	GetPolicy(ctx context.Context, id string) (*Policy, error)
	GetAgentPolicy(ctx context.Context, agentID string) (*Policy, error)

	// Run
	CreateRun(ctx context.Context, r *Run) error
	GetRunByID(ctx context.Context, id string) (*Run, error)
	UpdateRun(ctx context.Context, id string, update *RunUpdate) error
	ListRuns(ctx context.Context, filter *RunFilter) ([]*Run, error)

	// Action
	CreateAction(ctx context.Context, a *ActionRecord) error

	// Pricing
	GetPriceBook(ctx context.Context) (*PriceBook, error)
	SavePriceBook(ctx context.Context, book *PriceBook) error

	// Budget
	InsertBudgetEntry(ctx context.Context, entry *BudgetEntry) error
	AddRunSpend(ctx context.Context, runID string, costUSD float64) error
	SumAgentSpend(ctx context.Context, agentID string, sinceMs int64) (float64, error)
	GetSpendReport(ctx context.Context, filter *SpendFilter) (*SpendReport, error)

	// Tokens
	InsertAgentToken(ctx context.Context, token *AgentToken) error
	IsTokenRevoked(ctx context.Context, tokenID string) (bool, error)
	InsertRevokedToken(ctx context.Context, tokenID string) error

	// Credentials
	GetCredential(ctx context.Context, workspaceID, connector string) (*Credential, error)
	StoreCredential(ctx context.Context, c *Credential) error
	DeleteCredential(ctx context.Context, id string) error

	// Lifecycle
	WithTx(ctx context.Context, fn func(AppStore) error) error
	Migrate(ctx context.Context) error
	Close() error
}

// SpanStore holds trace spans. Separated from AppStore because it uses a
// different backend optimized for append-heavy analytical queries (DuckDB by default).
type SpanStore interface {
	WriteSpan(ctx context.Context, span *SpanRow) error
	UpdateSpan(ctx context.Context, span *SpanRow) error
	GetSpan(ctx context.Context, spanID string) (*SpanRow, error)
	QuerySpans(ctx context.Context, filter *SpanFilter) ([]*SpanRow, error)
	SpendByDimension(ctx context.Context, groupBy string, filter *SpanFilter) (map[string]float64, error)
	Migrate(ctx context.Context) error
	Close() error
}

// ── Data types ────────────────────────────────────────────────────────────────

// Workspace is the multi-tenancy root.
type Workspace struct {
	ID          string
	Name        string
	Slug        string
	Description string
	CreatedAt   int64 // UnixMilli
	UpdatedAt   int64 // UnixMilli
}

// Agent is a registered AI agent identity.
type Agent struct {
	ID            string
	WorkspaceID   string
	Name          string
	Description   string
	PolicyID      *string
	MonthlyBudget *float64
	Metadata      map[string]any
	CreatedAt     int64 // UnixMilli
	UpdatedAt     int64 // UnixMilli
}

// AgentUpdate holds partial update fields for an agent. Nil fields are not updated.
type AgentUpdate struct {
	Description   *string
	PolicyID      *string
	MonthlyBudget *float64
	Metadata      map[string]any
}

// Policy defines what an agent is allowed to do.
type Policy struct {
	ID                string
	WorkspaceID       string
	Name              string
	Description       string
	AllowedConnectors []string
	AllowedMethods    []string
	BudgetCapUSD      float64
	Config            map[string]any
	CreatedAt         int64 // UnixMilli
	UpdatedAt         int64 // UnixMilli
}

// Run represents one agent task execution from start to finish.
type Run struct {
	ID           string
	WorkspaceID  string
	AgentID      string
	PolicyID     *string
	Name         string
	Status       string
	BudgetCapUSD float64
	SpentUSD     float64
	Metadata     map[string]any
	ErrorMessage *string
	StartedAt    int64  // UnixMilli
	EndedAt      *int64 // UnixMilli; nil if still running
	CreatedAt    int64  // UnixMilli
	UpdatedAt    int64  // UnixMilli
}

// RunUpdate holds partial update fields for a run. Nil fields are not updated.
type RunUpdate struct {
	Status       *string
	SpentUSD     *float64
	ErrorMessage *string
	EndedAt      *int64
	Metadata     map[string]any
}

// RunFilter filters ListRuns queries.
type RunFilter struct {
	WorkspaceID string
	AgentID     string
	Status      string
	FromMs      *int64
	ToMs        *int64
	Limit       int
}

// ActionRecord is the persisted representation of an intercepted action.
type ActionRecord struct {
	ID         string
	RunID      string
	ActionType string
	Connector  string
	Method     string
	Input      []byte
	Metadata   map[string]any
	CreatedAt  int64 // UnixMilli
}

// PriceSnapshot captures the exact price inputs used to compute a cost.
// Snapshots are stored with budget entries and spans so historical cost remains stable.
type PriceSnapshot struct {
	Version              string
	Provider             string
	Model                string
	Match                string
	Source               string
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
	ResolvedAt           int64 // UnixMilli
}

// PriceModel defines one provider/model pricing rule.
type PriceModel struct {
	Provider             string
	Match                string
	Source               string
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

// PriceBook is the active pricing configuration used for metering.
type PriceBook struct {
	Version string
	Entries []PriceModel
}

// BudgetEntry is one record in the append-only budget ledger.
type BudgetEntry struct {
	ID               string
	WorkspaceID      string
	AgentID          string
	RunID            string
	ActionID         *string
	SpanID           *string
	Connector        string
	Model            string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CostUSD          float64
	PriceVersion     string
	PriceSnapshot    *PriceSnapshot
	Metadata         map[string]any
	CreatedAt        int64 // UnixMilli
}

// SpendFilter filters spend queries.
type SpendFilter struct {
	AgentID   string
	Connector string
	Model     string
	FromMs    *int64
	ToMs      *int64
}

// SpendReport is the result of an aggregated spend query.
type SpendReport struct {
	TotalUSD    float64
	ByAgent     map[string]float64
	ByConnector map[string]float64
	ByModel     map[string]float64
	PeriodStart int64 // UnixMilli
	PeriodEnd   int64 // UnixMilli
}

// AgentToken is an authorization token issued to an agent.
type AgentToken struct {
	ID           string
	AgentID      string
	Connectors   []string
	Methods      []string
	BudgetCapUSD *float64
	ExpiresAt    int64 // UnixMilli
	CreatedAt    int64 // UnixMilli
}

// Credential holds an encrypted API key for a connector.
type Credential struct {
	ID          string
	WorkspaceID string
	Connector   string
	Label       string
	KeyHash     string
	Encrypted   []byte
	LastUsedAt  *int64 // UnixMilli
	CreatedAt   int64  // UnixMilli
}

// SpanRow is the flat database representation of a trace span.
type SpanRow struct {
	ID               string
	RunID            string
	ActionID         string
	ParentID         *string
	Name             string
	StartedAt        int64  // UnixMilli
	EndedAt          *int64 // UnixMilli
	DurationMs       int64
	Input            []byte
	Output           []byte
	Tags             []byte
	Error            *string
	InputTokens      *int
	OutputTokens     *int
	CacheReadTokens  *int
	CacheWriteTokens *int
	Model            *string
	CostUSD          *float64
	PriceVersion     *string
	PriceSnapshot    *PriceSnapshot
	CreatedAt        int64 // UnixMilli
}

// SpanFilter filters QuerySpans queries.
type SpanFilter struct {
	RunID    string
	ActionID string
	FromMs   *int64
	ToMs     *int64
	HasError *bool
	Limit    int
}
