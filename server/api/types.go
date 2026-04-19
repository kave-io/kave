package api

import (
	"github.com/kave-io/kave/core/model/control"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/server/internal/contract"
)

// Agent is the API representation of a control.Agent.
type Agent struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	EnvID         string          `json:"env_id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	PolicyID      *string         `json:"policy_id"`
	MonthlyBudget *contract.Money `json:"monthly_budget"`
	Status        string          `json:"status"`
	Metadata      map[string]any  `json:"metadata"`
	CreatedAt     string          `json:"created_at"`
	CreatedAtMS   int64           `json:"created_at_ms"`
	UpdatedAt     string          `json:"updated_at"`
	UpdatedAtMS   int64           `json:"updated_at_ms"`
	DeletedAt     *string         `json:"deleted_at"`
	DeletedAtMS   *int64          `json:"deleted_at_ms"`
}

// Policy is the API representation of a control.PolicyRecord.
type Policy struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"project_id"`
	EnvID             string          `json:"env_id"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	AllowedTypes      []string        `json:"allowed_types"`
	AllowedConnectors []string        `json:"allowed_connectors"`
	AllowedMethods    []string        `json:"allowed_methods"`
	BudgetCap         *contract.Money `json:"budget_cap"`
	BudgetPeriod      string          `json:"budget_period"`
	BudgetBehavior    string          `json:"budget_behavior"`
	TraceInput        bool            `json:"trace_input"`
	TraceOutput       bool            `json:"trace_output"`
	RetentionDays     int             `json:"retention_days"`
	Mode              string          `json:"mode"`
	Status            string          `json:"status"`
	Config            map[string]any  `json:"config"`
	CreatedAt         string          `json:"created_at"`
	CreatedAtMS       int64           `json:"created_at_ms"`
	UpdatedAt         string          `json:"updated_at"`
	UpdatedAtMS       int64           `json:"updated_at_ms"`
}

// Run is the API representation of a runtime.RunRecord.
type Run struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"project_id"`
	EnvID        string          `json:"env_id"`
	AgentID      string          `json:"agent_id"`
	PolicyID     *string         `json:"policy_id"`
	Name         string          `json:"name"`
	Status       string          `json:"status"`
	BudgetCap    *contract.Money `json:"budget_cap"`
	Spent        *contract.Money `json:"spent"`
	Metadata     map[string]any  `json:"metadata"`
	ErrorMessage *string         `json:"error_message"`
	StartedAt    string          `json:"started_at"`
	StartedAtMS  int64           `json:"started_at_ms"`
	EndedAt      *string         `json:"ended_at"`
	EndedAtMS    *int64          `json:"ended_at_ms"`
	CreatedAt    string          `json:"created_at"`
	CreatedAtMS  int64           `json:"created_at_ms"`
	UpdatedAt    string          `json:"updated_at"`
	UpdatedAtMS  int64           `json:"updated_at_ms"`
}

// Span is the API representation of a runtime.SpanRow.
type Span struct {
	ID               string          `json:"id"`
	RunID            string          `json:"run_id"`
	ActionID         string          `json:"action_id"`
	ParentID         *string         `json:"parent_id"`
	Name             string          `json:"name"`
	StartedAt        string          `json:"started_at"`
	StartedAtMS      int64           `json:"started_at_ms"`
	EndedAt          *string         `json:"ended_at"`
	EndedAtMS        *int64          `json:"ended_at_ms"`
	DurationMs       int64           `json:"duration_ms"`
	Error            *string         `json:"error"`
	InputTokens      *int            `json:"input_tokens"`
	OutputTokens     *int            `json:"output_tokens"`
	CacheReadTokens  *int            `json:"cache_read_tokens"`
	CacheWriteTokens *int            `json:"cache_write_tokens"`
	Model            *string         `json:"model"`
	Cost             *contract.Money `json:"cost"`
	CreatedAt        string          `json:"created_at"`
	CreatedAtMS      int64           `json:"created_at_ms"`
}

// SpendReport is the API representation of a runtime.SpendReport.
type SpendReport struct {
	Total         contract.Money    `json:"total"`
	ByAgent       map[string]string `json:"by_agent"`
	ByConnector   map[string]string `json:"by_connector"`
	ByModel       map[string]string `json:"by_model"`
	PeriodStart   string            `json:"period_start"`
	PeriodStartMS int64             `json:"period_start_ms"`
	PeriodEnd     string            `json:"period_end"`
	PeriodEndMS   int64             `json:"period_end_ms"`
}

// PriceModel is a pricing entry.
type PriceModel struct {
	Provider             string `json:"provider"`
	Match                string `json:"match"`
	Source               string `json:"source"`
	Currency             string `json:"currency"`
	InputPerMillion      string `json:"input_per_million"`
	OutputPerMillion     string `json:"output_per_million"`
	CacheReadPerMillion  string `json:"cache_read_per_million"`
	CacheWritePerMillion string `json:"cache_write_per_million"`
}

// PriceBook is the pricing configuration.
type PriceBook struct {
	Version string       `json:"version"`
	Entries []PriceModel `json:"entries"`
}

// CreateAgentRequest is the request to create an agent.
type CreateAgentRequest struct {
	ProjectID     string         `json:"project_id"`
	EnvID         string         `json:"env_id"`
	Name          string         `json:"name"`
	Description   *string        `json:"description,omitempty"`
	PolicyID      *string        `json:"policy_id,omitempty"`
	MonthlyBudget *string        `json:"monthly_budget,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// CreatePolicyRequest is the request to create a policy.
type CreatePolicyRequest struct {
	ProjectID         string         `json:"project_id"`
	EnvID             string         `json:"env_id"`
	Name              string         `json:"name"`
	Description       *string        `json:"description,omitempty"`
	AllowedTypes      []string       `json:"allowed_types,omitempty"`
	AllowedConnectors []string       `json:"allowed_connectors,omitempty"`
	AllowedMethods    []string       `json:"allowed_methods,omitempty"`
	Config            map[string]any `json:"config,omitempty"`
}

// UpdateAgentRequest is the request to update an agent.
type UpdateAgentRequest struct {
	Name          *string        `json:"name,omitempty"`
	Description   *string        `json:"description,omitempty"`
	PolicyID      *string        `json:"policy_id,omitempty"`
	MonthlyBudget *string        `json:"monthly_budget,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// MapAgentToAPI converts a control.Agent to API Agent.
func MapAgentToAPI(a *control.Agent) *Agent {
	api := &Agent{
		ID:          a.ID,
		ProjectID:   a.ProjectID,
		EnvID:       a.EnvID,
		Name:        a.Name,
		Description: a.Description,
		PolicyID:    a.PolicyID,
		Status:      a.Status,
		Metadata:    a.Metadata,
		CreatedAt:   isoFromMS(a.CreatedAt),
		CreatedAtMS: a.CreatedAt,
		UpdatedAt:   isoFromMS(a.UpdatedAt),
		UpdatedAtMS: a.UpdatedAt,
		DeletedAt:   isoFromMSPtr(a.DeletedAt),
		DeletedAtMS: a.DeletedAt,
	}
	if a.MonthlyBudget != nil {
		api.MonthlyBudget = &contract.Money{Amount: a.MonthlyBudget.String(), Currency: apiDefaultCurrency}
	}
	if a.Metadata == nil {
		api.Metadata = make(map[string]any)
	}
	return api
}

// MapPolicyToAPI converts a control.PolicyRecord to API Policy.
func MapPolicyToAPI(p *control.PolicyRecord) *Policy {
	api := &Policy{
		ID:                p.ID,
		ProjectID:         p.ProjectID,
		EnvID:             p.EnvID,
		Name:              p.Name,
		Description:       p.Description,
		AllowedTypes:      p.AllowedTypes,
		AllowedConnectors: p.AllowedConnectors,
		AllowedMethods:    p.AllowedMethods,
		BudgetPeriod:      p.BudgetPeriod,
		BudgetBehavior:    p.BudgetBehavior,
		TraceInput:        p.TraceInput,
		TraceOutput:       p.TraceOutput,
		RetentionDays:     p.RetentionDays,
		Mode:              p.Mode,
		Status:            p.Status,
		Config:            p.Config,
		CreatedAt:         isoFromMS(p.CreatedAt),
		CreatedAtMS:       p.CreatedAt,
		UpdatedAt:         isoFromMS(p.UpdatedAt),
		UpdatedAtMS:       p.UpdatedAt,
	}
	if api.AllowedTypes == nil {
		api.AllowedTypes = []string{}
	}
	if api.AllowedConnectors == nil {
		api.AllowedConnectors = []string{}
	}
	if api.AllowedMethods == nil {
		api.AllowedMethods = []string{}
	}
	if api.Config == nil {
		api.Config = map[string]any{}
	}
	if p.BudgetCap > 0 {
		api.BudgetCap = &contract.Money{Amount: p.BudgetCap.String(), Currency: apiDefaultCurrency}
	}
	return api
}

// MapRunToAPI converts a runtime.RunRecord to API Run.
func MapRunToAPI(r *runtimemodel.RunRecord) *Run {
	api := &Run{
		ID:           r.ID,
		ProjectID:    r.ProjectID,
		EnvID:        r.EnvID,
		AgentID:      r.AgentID,
		PolicyID:     r.PolicyID,
		Name:         r.Name,
		Status:       r.Status,
		Metadata:     r.Metadata,
		ErrorMessage: r.ErrorMessage,
		StartedAt:    isoFromMS(r.StartedAt),
		StartedAtMS:  r.StartedAt,
		EndedAt:      isoFromMSPtr(r.EndedAt),
		EndedAtMS:    r.EndedAt,
		CreatedAt:    isoFromMS(r.CreatedAt),
		CreatedAtMS:  r.CreatedAt,
		UpdatedAt:    isoFromMS(r.UpdatedAt),
		UpdatedAtMS:  r.UpdatedAt,
	}
	if r.BudgetCap > 0 {
		api.BudgetCap = &contract.Money{Amount: r.BudgetCap.String(), Currency: apiDefaultCurrency}
	}
	if r.Spent > 0 {
		api.Spent = &contract.Money{Amount: r.Spent.String(), Currency: apiDefaultCurrency}
	}
	if r.Metadata == nil {
		api.Metadata = make(map[string]any)
	}
	return api
}

// MapSpanRowToAPI converts a runtime.SpanRow to API Span.
func MapSpanRowToAPI(s *runtimemodel.SpanRow) *Span {
	api := &Span{
		ID:               s.ID,
		RunID:            s.RunID,
		ActionID:         s.ActionID,
		ParentID:         s.ParentID,
		Name:             s.Name,
		StartedAt:        isoFromMS(s.StartedAt),
		StartedAtMS:      s.StartedAt,
		EndedAt:          isoFromMSPtr(s.EndedAt),
		EndedAtMS:        s.EndedAt,
		DurationMs:       s.DurationMs,
		Error:            s.Error,
		InputTokens:      s.InputTokens,
		OutputTokens:     s.OutputTokens,
		CacheReadTokens:  s.CacheReadTokens,
		CacheWriteTokens: s.CacheWriteTokens,
		Model:            s.Model,
		CreatedAt:        isoFromMS(s.CreatedAt),
		CreatedAtMS:      s.CreatedAt,
	}
	if s.Cost != nil {
		api.Cost = &contract.Money{Amount: s.Cost.String(), Currency: apiDefaultCurrency}
	}
	return api
}
