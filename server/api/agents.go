package api

import (
	"encoding/json"
	"net/http"

	"github.com/kave-io/kave/core/model/control"
	"github.com/kave-io/kave/core/pkg/money"
)

// listAgents returns all agents in an environment.
func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	envID := getQueryParam(r, "env_id")
	if envID == "" {
		errorJSON(w, http.StatusBadRequest, "env_id query parameter required")
		return
	}

	page := pageQuery(r)
	agents, err := h.app.ListAgents(ctx, envID, page)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]*Agent, 0, len(agents.Items))
	for _, a := range agents.Items {
		result = append(result, MapAgentToAPI(a, nil))
	}

	responseJSON(w, http.StatusOK, result)
}

// getAgent returns a single agent.
func (h *Handler) getAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := getPathParam(r, "id")

	agent, err := h.app.GetAgentByID(ctx, id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agent == nil {
		errorJSON(w, http.StatusNotFound, "agent not found")
		return
	}

	responseJSON(w, http.StatusOK, MapAgentToAPI(agent, nil))
}

// createAgent creates a new agent.
func (h *Handler) createAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.ProjectID == "" || req.EnvID == "" {
		errorJSON(w, http.StatusBadRequest, "name, project_id, and env_id required")
		return
	}

	now := getCurrentTimeMs()
	agent := &control.Agent{
		ID:          generateID(),
		ProjectID:   req.ProjectID,
		EnvID:       req.EnvID,
		Name:        req.Name,
		Description: stringOrEmpty(req.Description),
		PolicyID:    req.PolicyID,
		Status:      "active",
		Metadata:    req.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if req.MonthlyBudget != nil {
		budget, err := money.ParseAmount(*req.MonthlyBudget)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, "invalid monthly_budget format")
			return
		}
		agent.MonthlyBudget = &budget
	}

	if err := h.app.CreateAgent(ctx, agent); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	responseJSON(w, http.StatusCreated, MapAgentToAPI(agent, nil))
}

// updateAgent updates an existing agent.
func (h *Handler) updateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := getPathParam(r, "id")

	agent, err := h.app.GetAgentByID(ctx, id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agent == nil {
		errorJSON(w, http.StatusNotFound, "agent not found")
		return
	}

	var req UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}

	update := &control.AgentUpdate{
		Description: req.Description,
		PolicyID:    req.PolicyID,
		Status:      nil,
		Metadata:    req.Metadata,
	}

	if req.MonthlyBudget != nil {
		budget, err := money.ParseAmount(*req.MonthlyBudget)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, "invalid monthly_budget format")
			return
		}
		update.MonthlyBudget = &budget
	}

	if err := h.app.UpdateAgent(ctx, id, update); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch the updated agent to return
	updated, err := h.app.GetAgentByID(ctx, id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	responseJSON(w, http.StatusOK, MapAgentToAPI(updated, nil))
}
