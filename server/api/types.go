package api

import (
	"github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
)

// Agent is the API representation of a control.Agent.
type Agent struct {
	ID                string            `json:"id"`
	ProjectID         string            `json:"project_id"`
	EnvID             string            `json:"env_id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	PolicyID          *string           `json:"policy_id,omitempty"`
	MonthlyBudget     *string           `json:"monthly_budget,omitempty"`
	MonthlyBudgetDisplay *DisplayMoney  `json:"monthly_budget_display,omitempty"`
	Status            string            `json:"status"`
	Metadata          map[string]any    `json:"metadata"`
	CreatedAt         int64             `json:"created_at"`
	UpdatedAt         int64             `json:"updated_at"`
}

// Policy is the API representation of a control.PolicyRecord.
type Policy struct {
	ID               string            `json:"id"`
	ProjectID        string            `json:"project_id"`
	EnvID            string            `json:"env_id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	AllowedTypes     []string          `json:"allowed_types"`
	AllowedConnectors []string         `json:"allowed_connectors"`
	AllowedMethods   []string          `json:"allowed_methods"`
	BudgetCap        *string           `json:"budget_cap,omitempty"`
	BudgetCapDisplay *DisplayMoney     `json:"budget_cap_display,omitempty"`
	BudgetPeriod     string            `json:"budget_period"`
	BudgetBehavior   string            `json:"budget_behavior"`
	TraceInput       bool              `json:"trace_input"`
	TraceOutput      bool              `json:"trace_output"`
	RetentionDays    int               `json:"retention_days"`
	Mode             string            `json:"mode"`
	Status           string            `json:"status"`
	Config           map[string]any    `json:"config"`
	CreatedAt        int64             `json:"created_at"`
	UpdatedAt        int64             `json:"updated_at"`
}

// Run is the API representation of a runtime.RunRecord.
type Run struct {
	ID            string            `json:"id"`
	ProjectID     string            `json:"project_id"`
	EnvID         string            `json:"env_id"`
	AgentID       string            `json:"agent_id"`
	PolicyID      *string           `json:"policy_id,omitempty"`
	Name          string            `json:"name"`
	Status        string            `json:"status"`
	BudgetCap     *string           `json:"budget_cap,omitempty"`
	BudgetCapDisplay *DisplayMoney  `json:"budget_cap_display,omitempty"`
	Spent         *string           `json:"spent,omitempty"`
	SpentDisplay  *DisplayMoney     `json:"spent_display,omitempty"`
	Metadata      map[string]any    `json:"metadata"`
	ErrorMessage  *string           `json:"error_message,omitempty"`
	StartedAt     int64             `json:"started_at"`
	EndedAt       *int64            `json:"ended_at,omitempty"`
	CreatedAt     int64             `json:"created_at"`
	UpdatedAt     int64             `json:"updated_at"`
}

// Span is the API representation of a runtime.SpanRow.
type Span struct {
	ID               string            `json:"id"`
	RunID            string            `json:"run_id"`
	ActionID         string            `json:"action_id"`
	ParentID         *string           `json:"parent_id,omitempty"`
	Name             string            `json:"name"`
	StartedAt        int64             `json:"started_at"`
	EndedAt          *int64            `json:"ended_at,omitempty"`
	DurationMs       int64             `json:"duration_ms"`
	Error            *string           `json:"error,omitempty"`
	InputTokens      *int              `json:"input_tokens,omitempty"`
	OutputTokens     *int              `json:"output_tokens,omitempty"`
	CacheReadTokens  *int              `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int              `json:"cache_write_tokens,omitempty"`
	Model            *string           `json:"model,omitempty"`
	Cost             *string           `json:"cost,omitempty"`
	CostDisplay      *DisplayMoney     `json:"cost_display,omitempty"`
	CreatedAt        int64             `json:"created_at"`
}

// DisplayMoney represents money with currency and formatting.
type DisplayMoney struct {
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	FXRate        *string `json:"fx_rate,omitempty"`
	FXSource      *string `json:"fx_source,omitempty"`
	FXAsOfDate    *string `json:"fx_as_of_date,omitempty"`
	FXFetchedAt   *int64 `json:"fx_fetched_at,omitempty"`
	Rounded       bool   `json:"rounded"`
}

// SpendReport is the API representation of a runtime.SpendReport.
type SpendReport struct {
	Total           string                `json:"total"`
	TotalDisplay    *DisplayMoney         `json:"total_display,omitempty"`
	ByAgent         map[string]string     `json:"by_agent"`
	ByConnector     map[string]string     `json:"by_connector"`
	ByModel         map[string]string     `json:"by_model"`
	PeriodStart     int64                 `json:"period_start"`
	PeriodEnd       int64                 `json:"period_end"`
}

// PriceModel is a pricing entry.
type PriceModel struct {
	Provider            string `json:"provider"`
	Match               string `json:"match"`
	Source              string `json:"source"`
	Currency            string `json:"currency"`
	InputPerMillion     string `json:"input_per_million"`
	OutputPerMillion    string `json:"output_per_million"`
	CacheReadPerMillion string `json:"cache_read_per_million"`
	CacheWritePerMillion string `json:"cache_write_per_million"`
}

// PriceBook is the pricing configuration.
type PriceBook struct {
	Version string       `json:"version"`
	Entries []PriceModel `json:"entries"`
}

// CreateAgentRequest is the request to create an agent.
type CreateAgentRequest struct {
	ProjectID     string            `json:"project_id"`
	EnvID         string            `json:"env_id"`
	Name          string            `json:"name"`
	Description   *string           `json:"description,omitempty"`
	PolicyID      *string           `json:"policy_id,omitempty"`
	MonthlyBudget *string           `json:"monthly_budget,omitempty"`
	Metadata      map[string]any    `json:"metadata,omitempty"`
}

// CreatePolicyRequest is the request to create a policy.
type CreatePolicyRequest struct {
	ProjectID        string            `json:"project_id"`
	EnvID            string            `json:"env_id"`
	Name             string            `json:"name"`
	Description      *string           `json:"description,omitempty"`
	AllowedTypes     []string          `json:"allowed_types,omitempty"`
	AllowedConnectors []string         `json:"allowed_connectors,omitempty"`
	AllowedMethods   []string          `json:"allowed_methods,omitempty"`
	Config           map[string]any    `json:"config,omitempty"`
}

// UpdateAgentRequest is the request to update an agent.
type UpdateAgentRequest struct {
	Name          *string           `json:"name,omitempty"`
	Description   *string           `json:"description,omitempty"`
	PolicyID      *string           `json:"policy_id,omitempty"`
	MonthlyBudget *string           `json:"monthly_budget,omitempty"`
	Metadata      map[string]any    `json:"metadata,omitempty"`
}

// MapAgentToAPI converts a control.Agent to API Agent.
func MapAgentToAPI(a *control.Agent, displayBudget *DisplayMoney) *Agent {
	api := &Agent{
		ID:          a.ID,
		ProjectID:   a.ProjectID,
		EnvID:       a.EnvID,
		Name:        a.Name,
		Description: a.Description,
		PolicyID:    a.PolicyID,
		Status:      a.Status,
		Metadata:    a.Metadata,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
		MonthlyBudgetDisplay: displayBudget,
	}
	if a.MonthlyBudget != nil {
		s := a.MonthlyBudget.String()
		api.MonthlyBudget = &s
	}
	if a.Metadata == nil {
		api.Metadata = make(map[string]any)
	}
	return api
}

// MapPolicyToAPI converts a control.PolicyRecord to API Policy.
func MapPolicyToAPI(p *control.PolicyRecord) *Policy {
	api := &Policy{
		ID:               p.ID,
		ProjectID:        p.ProjectID,
		EnvID:            p.EnvID,
		Name:             p.Name,
		Description:      p.Description,
		AllowedTypes:     p.AllowedTypes,
		AllowedConnectors: p.AllowedConnectors,
		AllowedMethods:   p.AllowedMethods,
		BudgetPeriod:     p.BudgetPeriod,
		BudgetBehavior:   p.BudgetBehavior,
		TraceInput:       p.TraceInput,
		TraceOutput:      p.TraceOutput,
		RetentionDays:    p.RetentionDays,
		Mode:             p.Mode,
		Status:           p.Status,
		Config:           p.Config,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
	if p.BudgetCap > 0 {
		s := p.BudgetCap.String()
		api.BudgetCap = &s
	}
	return api
}

// MapRunToAPI converts a runtime.RunRecord to API Run.
func MapRunToAPI(r *runtimemodel.RunRecord, budgetCapDisplay *DisplayMoney, spentDisplay *DisplayMoney) *Run {
	api := &Run{
		ID:               r.ID,
		ProjectID:        r.ProjectID,
		EnvID:            r.EnvID,
		AgentID:          r.AgentID,
		PolicyID:         r.PolicyID,
		Name:             r.Name,
		Status:           r.Status,
		Metadata:         r.Metadata,
		ErrorMessage:     r.ErrorMessage,
		StartedAt:        r.StartedAt,
		EndedAt:          r.EndedAt,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		BudgetCapDisplay: budgetCapDisplay,
		SpentDisplay:     spentDisplay,
	}
	if r.BudgetCap > 0 {
		s := r.BudgetCap.String()
		api.BudgetCap = &s
	}
	if r.Spent > 0 {
		s := r.Spent.String()
		api.Spent = &s
	}
	if r.Metadata == nil {
		api.Metadata = make(map[string]any)
	}
	return api
}

// MapSpanRowToAPI converts a runtime.SpanRow to API Span.
func MapSpanRowToAPI(s *runtimemodel.SpanRow, costDisplay *DisplayMoney) *Span {
	api := &Span{
		ID:          s.ID,
		RunID:       s.RunID,
		ActionID:    s.ActionID,
		ParentID:    s.ParentID,
		Name:        s.Name,
		StartedAt:   s.StartedAt,
		EndedAt:     s.EndedAt,
		DurationMs:  s.DurationMs,
		Error:       s.Error,
		InputTokens: s.InputTokens,
		OutputTokens: s.OutputTokens,
		CacheReadTokens: s.CacheReadTokens,
		CacheWriteTokens: s.CacheWriteTokens,
		Model:       s.Model,
		CreatedAt:   s.CreatedAt,
		CostDisplay: costDisplay,
	}
	if s.Cost != nil {
		str := s.Cost.String()
		api.Cost = &str
	}
	return api
}
