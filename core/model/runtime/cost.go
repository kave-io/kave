package runtime

import "github.com/kave-io/kave/core/pkg/money"

// PriceSnapshot captures the exact price inputs used to compute a cost.
// Prices are float64 — they are config/reference values, not accounting values.
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
	ReasoningPerMillion  float64
	AudioInputPerMillion float64
	AudioOutputPerMillion float64
	ImageUnitPrice       float64
	PerRequest           float64
	PerComputeMs         float64
	PerGBStored          float64
	PerGBTransferred     float64
	ResolvedAt           int64 // UnixMilli
}

// PriceModel defines one provider/model pricing rule.
// Prices are float64 — reference config, not accounting.
type PriceModel struct {
	Provider              string
	Match                 string
	Source                string
	InputPerMillion       float64
	OutputPerMillion      float64
	CacheReadPerMillion   float64
	CacheWritePerMillion  float64
	ReasoningPerMillion   float64  // reasoning tokens priced separately on some models
	AudioInputPerMillion  float64  // audio input tokens (OpenAI audio models)
	AudioOutputPerMillion float64  // audio output tokens (OpenAI audio models)
	ImageUnitPrice        float64  // per image unit (tile/call/etc. — provider-specific)
	PerRequest            float64  // per API call (some tools charge per-request)
	PerComputeMs          float64  // compute time (Replicate, Modal, RunPod)
	PerGBStored           float64  // storage cost per GB-month
	PerGBTransferred      float64  // bandwidth cost per GB
	EffectiveFrom         int64    // UnixMilli; 0 = from beginning of time
	EffectiveTo           *int64   // UnixMilli; nil = currently active
	RevisionNote          string   // brief change description
}

// PriceBook is the active pricing configuration used for metering.
type PriceBook struct {
	Version string
	Entries []PriceModel
}

// BudgetEntry is one record in the append-only budget ledger.
type BudgetEntry struct {
	ID                  string
	ProjectID           string
	EnvID               string
	PolicyID            string // which policy governed this spend
	AgentID             string
	RunID               string
	ActionID            *string
	SpanID              *string
	Connector           string
	Model               string
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheWriteTokens    int
	ReasoningTokens     int
	AudioInputTokens    int
	AudioOutputTokens   int
	ImageUnits          int
	RequestCount        int
	ComputeMs           int64
	StorageBytes        int64
	BandwidthBytes      int64
	Cost                money.Amount
	PriceVersion        string
	PriceSnapshot       *PriceSnapshot
	UsageDetail         map[string]any // serialized Usage struct (non-token fields)
	Metadata            map[string]any
	CreatedAt           int64 // UnixMilli
}

// SpendFilter filters spend queries.
type SpendFilter struct {
	// Scope filters (at least one required for meaningful results)
	ProjectID string // filter by project/workspace
	EnvID     string // filter by environment
	PolicyID  string // filter by policy
	AgentID   string // filter by agent
	Connector string // filter by connector
	Model     string // filter by model

	// Time range
	FromMs *int64
	ToMs   *int64
}

// SpendReport is the result of an aggregated spend query.
// Includes multi-dimensional grouping for business reporting.
type SpendReport struct {
	Total money.Amount

	// Workspace and environment breakdown
	ByProject map[string]money.Amount // group by project ID
	ByEnv     map[string]money.Amount // group by environment ID

	// Policy and agent breakdown
	ByPolicy  map[string]money.Amount // group by policy ID
	ByAgent   map[string]money.Amount // group by agent ID

	// Operational breakdown
	ByConnector map[string]money.Amount // group by connector
	ByModel     map[string]money.Amount // group by model

	PeriodStart int64 // UnixMilli
	PeriodEnd   int64 // UnixMilli
}
