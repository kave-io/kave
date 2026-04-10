package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kave-io/kave/core/store"
)

type createPolicyReq struct {
	WorkspaceID       string         `json:"workspace_id"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	AllowedConnectors []string       `json:"allowed_connectors"`
	AllowedMethods    []string       `json:"allowed_methods"`
	BudgetCapUSD      float64        `json:"budget_cap_usd"`
	Config            map[string]any `json:"config"`
}

func (a *API) createPolicy(w http.ResponseWriter, r *http.Request) {
	var req createPolicyReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WorkspaceID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "workspace_id and name are required")
		return
	}

	now := time.Now().UnixMilli()
	policy := &store.Policy{
		ID:                uuid.NewString(),
		WorkspaceID:       req.WorkspaceID,
		Name:              req.Name,
		Description:       req.Description,
		AllowedConnectors: req.AllowedConnectors,
		AllowedMethods:    req.AllowedMethods,
		BudgetCapUSD:      req.BudgetCapUSD,
		Config:            req.Config,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := a.app.CreatePolicy(r.Context(), policy); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create policy: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, toPolicyResp(policy))
}

func (a *API) getPolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	policy, err := a.app.GetPolicy(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("get policy: %v", err))
		return
	}
	if policy == nil {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}
	writeJSON(w, http.StatusOK, toPolicyResp(policy))
}
