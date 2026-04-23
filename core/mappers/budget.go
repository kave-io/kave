package mappers

import (
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/runtime"
)

// BudgetEntryInput is the app-layer input for creating a budget ledger row.
type BudgetEntryInput struct {
	ID            string
	ProjectID     string
	EnvID         string
	AgentID       string
	RunID         string
	ActionID      *string
	SpanID        *string
	Connector     string
	TokenUsage    *runtime.TokenUsage
	Cost          money.Amount
	PriceVersion  string
	PriceSnapshot *runtimemodel.PriceSnapshot
	Blocked       bool
	BlockReason   string
	BlockPeriod   string
	CreatedAt     *int64
}

// BudgetEntryFromUsage converts token usage and price inputs to a persisted budget entry.
func BudgetEntryFromUsage(in *BudgetEntryInput) *runtimemodel.BudgetEntry {
	if in == nil {
		return nil
	}

	createdAt := msSinceEpoch()
	if in.CreatedAt != nil {
		createdAt = *in.CreatedAt
	}

	var usage runtime.TokenUsage
	if in.TokenUsage != nil {
		usage = *in.TokenUsage
	}

	return &runtimemodel.BudgetEntry{
		ID:               in.ID,
		ProjectID:        in.ProjectID,
		EnvID:            in.EnvID,
		AgentID:          in.AgentID,
		RunID:            in.RunID,
		ActionID:         in.ActionID,
		SpanID:           in.SpanID,
		Connector:        in.Connector,
		Model:            usage.Model,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheRead,
		CacheWriteTokens: usage.CacheWrite,
		Cost:             in.Cost,
		PriceVersion:     in.PriceVersion,
		PriceSnapshot:    in.PriceSnapshot,
		Blocked:          in.Blocked,
		BlockReason:      in.BlockReason,
		BlockPeriod:      in.BlockPeriod,
		CreatedAt:        createdAt,
	}
}

// BudgetEntryToTokenUsage extracts token usage data from a budget entry.
func BudgetEntryToTokenUsage(entry *runtimemodel.BudgetEntry) *runtime.TokenUsage {
	if entry == nil {
		return nil
	}
	return &runtime.TokenUsage{
		InputTokens:  entry.InputTokens,
		OutputTokens: entry.OutputTokens,
		CacheRead:    entry.CacheReadTokens,
		CacheWrite:   entry.CacheWriteTokens,
		Model:        entry.Model,
	}
}
