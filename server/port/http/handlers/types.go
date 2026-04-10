package handlers

import "github.com/kave-io/kave/core/store"

// API response types with snake_case json tags.
// These are the public contract — core store types stay tag-free.

type agentResp struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspace_id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	PolicyID      *string        `json:"policy_id,omitempty"`
	MonthlyBudget *float64       `json:"monthly_budget,omitempty"`
	Metadata      map[string]any `json:"metadata"`
	CreatedAt     int64          `json:"created_at"`
	UpdatedAt     int64          `json:"updated_at"`
}

type policyResp struct {
	ID                string         `json:"id"`
	WorkspaceID       string         `json:"workspace_id"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	AllowedConnectors []string       `json:"allowed_connectors"`
	AllowedMethods    []string       `json:"allowed_methods"`
	BudgetCapUSD      float64        `json:"budget_cap_usd"`
	Config            map[string]any `json:"config"`
	CreatedAt         int64          `json:"created_at"`
	UpdatedAt         int64          `json:"updated_at"`
}

type runResp struct {
	ID           string         `json:"id"`
	WorkspaceID  string         `json:"workspace_id"`
	AgentID      string         `json:"agent_id"`
	PolicyID     *string        `json:"policy_id,omitempty"`
	Name         string         `json:"name"`
	Status       string         `json:"status"`
	BudgetCapUSD float64        `json:"budget_cap_usd"`
	SpentUSD     float64        `json:"spent_usd"`
	Metadata     map[string]any `json:"metadata"`
	ErrorMessage *string        `json:"error_message,omitempty"`
	StartedAt    int64          `json:"started_at"`
	EndedAt      *int64         `json:"ended_at,omitempty"`
	CreatedAt    int64          `json:"created_at"`
	UpdatedAt    int64          `json:"updated_at"`
}

type spanResp struct {
	ID               string   `json:"id"`
	RunID            string   `json:"run_id"`
	ActionID         string   `json:"action_id"`
	ParentID         *string  `json:"parent_id,omitempty"`
	Name             string   `json:"name"`
	StartedAt        int64    `json:"started_at"`
	EndedAt          *int64   `json:"ended_at,omitempty"`
	DurationMs       int64    `json:"duration_ms"`
	Error            *string  `json:"error,omitempty"`
	InputTokens      *int     `json:"input_tokens,omitempty"`
	OutputTokens     *int     `json:"output_tokens,omitempty"`
	CacheReadTokens  *int     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int     `json:"cache_write_tokens,omitempty"`
	Model            *string  `json:"model,omitempty"`
	CostUSD          *float64 `json:"cost_usd,omitempty"`
	CreatedAt        int64    `json:"created_at"`
}

type spendResp struct {
	TotalUSD    float64            `json:"total_usd"`
	ByAgent     map[string]float64 `json:"by_agent"`
	ByConnector map[string]float64 `json:"by_connector"`
	ByModel     map[string]float64 `json:"by_model"`
	PeriodStart int64              `json:"period_start"`
	PeriodEnd   int64              `json:"period_end"`
}

// ── mappers ───────────────────────────────────────────────────────────────────

func toAgentResp(a *store.Agent) agentResp {
	return agentResp{
		ID:            a.ID,
		WorkspaceID:   a.WorkspaceID,
		Name:          a.Name,
		Description:   a.Description,
		PolicyID:      a.PolicyID,
		MonthlyBudget: a.MonthlyBudget,
		Metadata:      a.Metadata,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}

func toPolicyResp(p *store.Policy) policyResp {
	connectors := p.AllowedConnectors
	if connectors == nil {
		connectors = []string{}
	}
	methods := p.AllowedMethods
	if methods == nil {
		methods = []string{}
	}
	return policyResp{
		ID:                p.ID,
		WorkspaceID:       p.WorkspaceID,
		Name:              p.Name,
		Description:       p.Description,
		AllowedConnectors: connectors,
		AllowedMethods:    methods,
		BudgetCapUSD:      p.BudgetCapUSD,
		Config:            p.Config,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

func toRunResp(r *store.Run) runResp {
	return runResp{
		ID:           r.ID,
		WorkspaceID:  r.WorkspaceID,
		AgentID:      r.AgentID,
		PolicyID:     r.PolicyID,
		Name:         r.Name,
		Status:       r.Status,
		BudgetCapUSD: r.BudgetCapUSD,
		SpentUSD:     r.SpentUSD,
		Metadata:     r.Metadata,
		ErrorMessage: r.ErrorMessage,
		StartedAt:    r.StartedAt,
		EndedAt:      r.EndedAt,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

func toSpanResp(s *store.SpanRow) spanResp {
	return spanResp{
		ID:               s.ID,
		RunID:            s.RunID,
		ActionID:         s.ActionID,
		ParentID:         s.ParentID,
		Name:             s.Name,
		StartedAt:        s.StartedAt,
		EndedAt:          s.EndedAt,
		DurationMs:       s.DurationMs,
		Error:            s.Error,
		InputTokens:      s.InputTokens,
		OutputTokens:     s.OutputTokens,
		CacheReadTokens:  s.CacheReadTokens,
		CacheWriteTokens: s.CacheWriteTokens,
		Model:            s.Model,
		CostUSD:          s.CostUSD,
		CreatedAt:        s.CreatedAt,
	}
}

func toSpendResp(r *store.SpendReport) spendResp {
	byAgent := r.ByAgent
	if byAgent == nil {
		byAgent = map[string]float64{}
	}
	byConnector := r.ByConnector
	if byConnector == nil {
		byConnector = map[string]float64{}
	}
	byModel := r.ByModel
	if byModel == nil {
		byModel = map[string]float64{}
	}
	return spendResp{
		TotalUSD:    r.TotalUSD,
		ByAgent:     byAgent,
		ByConnector: byConnector,
		ByModel:     byModel,
		PeriodStart: r.PeriodStart,
		PeriodEnd:   r.PeriodEnd,
	}
}
