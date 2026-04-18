package runtime

import "github.com/kave-io/kave/core/pkg/money"

// DisplayMoney is a derived read-model amount for UI/reporting.
// It is never the canonical accounting value and should not be persisted as the
// source-of-truth in stores, logs, traces, or audits.
type DisplayMoney struct {
	Amount      money.Money
	FXRate      string
	FXSource    string
	FXAsOfDate  string
	FXFetchedAt int64
	Rounded     bool
}

// PriceSnapshot captures the exact price inputs used to compute a cost.
type PriceSnapshot struct {
	Version               string
	Provider              string
	Model                 string
	Match                 string
	Source                string
	Currency              money.CurrencyCode
	InputPerMillion       money.Amount
	OutputPerMillion      money.Amount
	CacheReadPerMillion   money.Amount
	CacheWritePerMillion  money.Amount
	ReasoningPerMillion   money.Amount
	AudioInputPerMillion  money.Amount
	AudioOutputPerMillion money.Amount
	ImageUnitPrice        money.Amount
	PerRequest            money.Amount
	PerComputeMs          money.Amount
	PerGBStored           money.Amount
	PerGBTransferred      money.Amount
	ResolvedAt            int64 // UnixMilli
}

// PriceModel is one entry in a PriceBook: pricing for a provider+model match,
// valid for a time window (EffectiveFrom, EffectiveTo).
type PriceModel struct {
	Provider              string            `json:"provider"`
	Match                 string            `json:"match"`
	Source                string            `json:"source"`
	Currency              money.CurrencyCode `json:"currency"`
	InputPerMillion       money.Amount      `json:"input_per_million"`
	OutputPerMillion      money.Amount      `json:"output_per_million"`
	CacheReadPerMillion   money.Amount      `json:"cache_read_per_million"`
	CacheWritePerMillion  money.Amount      `json:"cache_write_per_million"`
	ReasoningPerMillion   money.Amount      `json:"reasoning_per_million"`
	AudioInputPerMillion  money.Amount      `json:"audio_input_per_million"`
	AudioOutputPerMillion money.Amount      `json:"audio_output_per_million"`
	ImageUnitPrice        money.Amount      `json:"image_unit_price"`
	PerRequest            money.Amount      `json:"per_request"`
	PerComputeMs          money.Amount      `json:"per_compute_ms"`
	PerGBStored           money.Amount      `json:"per_gb_stored"`
	PerGBTransferred      money.Amount      `json:"per_gb_transferred"`
	EffectiveFrom         int64             `json:"effective_from"`
	EffectiveTo           *int64            `json:"effective_to"`
	RevisionNote          string            `json:"revision_note"`
}

// PriceBook is the active pricing configuration used for metering.
type PriceBook struct {
	Version string
	Entries []PriceModel
}

// BudgetEntry is one record in the append-only budget ledger.
type BudgetEntry struct {
	ID                string
	ProjectID         string
	EnvID             string
	PolicyID          string // which policy governed this spend
	AgentID           string
	RunID             string
	ActionID          *string
	SpanID            *string
	Connector         string
	Model             string
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheWriteTokens  int
	ReasoningTokens   int
	AudioInputTokens  int
	AudioOutputTokens int
	ImageUnits        int
	RequestCount      int
	ComputeMs         int64
	StorageBytes      int64
	BandwidthBytes    int64
	Cost              money.Amount
	PriceVersion      string
	PriceSnapshot     *PriceSnapshot
	UsageDetail       map[string]any // serialized Usage struct (non-token fields)
	Metadata          map[string]any
	CreatedAt         int64 // UnixMilli
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
	ByPolicy map[string]money.Amount // group by policy ID
	ByAgent  map[string]money.Amount // group by agent ID

	// Operational breakdown
	ByConnector map[string]money.Amount // group by connector
	ByModel     map[string]money.Amount // group by model

	PeriodStart int64 // UnixMilli
	PeriodEnd   int64 // UnixMilli
}
