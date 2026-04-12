package mappers

import (
	controlmodel "github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/money"
)

// AgentCreateInput is control-plane input for creating an agent.
type AgentCreateInput struct {
	ID            string
	ProjectID     string
	EnvID         string
	Name          string
	Description   string
	PolicyID      *string
	MonthlyBudget *money.Amount
	CreatedBy     string
	Metadata      map[string]any
	CreatedAt     *int64
	UpdatedAt     *int64
}

// AgentUpdateInput is control-plane input for patching an agent.
type AgentUpdateInput struct {
	Description   *string
	PolicyID      *string
	MonthlyBudget *money.Amount
	Status        *string
	Metadata      map[string]any
	UpdatedBy     *string
}

// AgentView is the app-layer response shape for an agent.
type AgentView struct {
	ID            string
	ProjectID     string
	EnvID         string
	Name          string
	Description   string
	PolicyID      *string
	MonthlyBudget *float64 // USD; nil = no budget
	Status        string
	Metadata      map[string]any
	CreatedBy     string
	UpdatedBy     string
	DeletedAt     *int64
	CreatedAt     int64
	UpdatedAt     int64
}

// AgentCreateToModel converts create input to controlmodel.Agent.
func AgentCreateToModel(in *AgentCreateInput) *controlmodel.Agent {
	if in == nil {
		return nil
	}

	createdAt := msSinceEpoch()
	if in.CreatedAt != nil {
		createdAt = *in.CreatedAt
	}
	updatedAt := createdAt
	if in.UpdatedAt != nil {
		updatedAt = *in.UpdatedAt
	}

	createdBy := in.CreatedBy
	if createdBy == "" {
		createdBy = "system"
	}

	return &controlmodel.Agent{
		ID:            in.ID,
		ProjectID:     in.ProjectID,
		EnvID:         in.EnvID,
		Name:          in.Name,
		Description:   in.Description,
		PolicyID:      in.PolicyID,
		MonthlyBudget: in.MonthlyBudget,
		Status:        controlmodel.AgentStatusActive,
		Metadata:      in.Metadata,
		CreatedBy:     createdBy,
		UpdatedBy:     createdBy,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}
}

// AgentUpdateToModel converts patch input to controlmodel.AgentUpdate.
func AgentUpdateToModel(in *AgentUpdateInput) *controlmodel.AgentUpdate {
	if in == nil {
		return nil
	}
	return &controlmodel.AgentUpdate{
		Description:   in.Description,
		PolicyID:      in.PolicyID,
		MonthlyBudget: in.MonthlyBudget,
		Status:        in.Status,
		Metadata:      in.Metadata,
		UpdatedBy:     in.UpdatedBy,
	}
}

// AgentToView converts a stored agent to the app-layer response shape.
func AgentToView(a *controlmodel.Agent) *AgentView {
	if a == nil {
		return nil
	}
	var budgetUSD *float64
	if a.MonthlyBudget != nil {
		v := a.MonthlyBudget.Dollars()
		budgetUSD = &v
	}
	return &AgentView{
		ID:            a.ID,
		ProjectID:     a.ProjectID,
		EnvID:         a.EnvID,
		Name:          a.Name,
		Description:   a.Description,
		PolicyID:      a.PolicyID,
		MonthlyBudget: budgetUSD,
		Status:        a.Status,
		Metadata:      a.Metadata,
		CreatedBy:     a.CreatedBy,
		UpdatedBy:     a.UpdatedBy,
		DeletedAt:     a.DeletedAt,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}
