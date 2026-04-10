package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kave-io/kave/core/store"
)

// store is used for Agent/AgentUpdate types; response types are in types.go

type createAgentReq struct {
	WorkspaceID   string         `json:"workspace_id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	PolicyID      *string        `json:"policy_id"`
	MonthlyBudget *float64       `json:"monthly_budget"`
	Metadata      map[string]any `json:"metadata"`
}

type updateAgentReq struct {
	Description   *string        `json:"description"`
	PolicyID      *string        `json:"policy_id"`
	MonthlyBudget *float64       `json:"monthly_budget"`
	Metadata      map[string]any `json:"metadata"`
}

func (a *API) createAgent(w http.ResponseWriter, r *http.Request) {
	var req createAgentReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorkspaceID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "workspace_id and name are required")
		return
	}

	budget := req.MonthlyBudget
	if budget == nil {
		def := 100.0
		budget = &def
	}

	now := time.Now().UnixMilli()
	agent := &store.Agent{
		ID:            uuid.NewString(),
		WorkspaceID:   req.WorkspaceID,
		Name:          req.Name,
		Description:   req.Description,
		PolicyID:      req.PolicyID,
		MonthlyBudget: budget,
		Metadata:      req.Metadata,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := a.app.CreateAgent(r.Context(), agent); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create agent: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, toAgentResp(agent))
}

func (a *API) getAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := a.app.GetAgentByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("get agent: %v", err))
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, toAgentResp(agent))
}

func (a *API) listAgents(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id query param is required")
		return
	}

	agents, err := a.app.ListAgents(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list agents: %v", err))
		return
	}

	out := make([]agentResp, len(agents))
	for i, ag := range agents {
		out[i] = toAgentResp(ag)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) updateAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req updateAgentReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	update := &store.AgentUpdate{
		Description:   req.Description,
		PolicyID:      req.PolicyID,
		MonthlyBudget: req.MonthlyBudget,
		Metadata:      req.Metadata,
	}

	if err := a.app.UpdateAgent(r.Context(), id, update); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("update agent: %v", err))
		return
	}

	agent, err := a.app.GetAgentByID(r.Context(), id)
	if err != nil || agent == nil {
		writeError(w, http.StatusNotFound, "agent not found after update")
		return
	}
	writeJSON(w, http.StatusOK, toAgentResp(agent))
}
