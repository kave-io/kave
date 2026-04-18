package api

import (
	"encoding/json"
	"net/http"

	"github.com/kave-io/kave/core/model/control"
)

// getPolicy returns a single policy.
func (h *Handler) getPolicy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := getPathParam(r, "id")

	policy, err := h.app.GetPolicy(ctx, id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if policy == nil {
		errorJSON(w, http.StatusNotFound, "policy not found")
		return
	}

	responseJSON(w, http.StatusOK, MapPolicyToAPI(policy))
}

// listPolicies returns all policies in an environment.
func (h *Handler) listPolicies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	envID := getQueryParam(r, "env_id")
	if envID == "" {
		errorJSON(w, http.StatusBadRequest, "env_id query parameter required")
		return
	}

	page := pageQuery(r)
	policies, err := h.app.ListPolicies(ctx, envID, page)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]*Policy, 0, len(policies.Items))
	for _, p := range policies.Items {
		result = append(result, MapPolicyToAPI(p))
	}

	responseJSON(w, http.StatusOK, result)
}

// createPolicy creates a new policy.
func (h *Handler) createPolicy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req CreatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.ProjectID == "" || req.EnvID == "" {
		errorJSON(w, http.StatusBadRequest, "name, project_id, and env_id required")
		return
	}

	now := getCurrentTimeMs()
	policy := &control.PolicyRecord{
		ID:                generateID(),
		ProjectID:         req.ProjectID,
		EnvID:             req.EnvID,
		Name:              req.Name,
		Description:       stringOrEmpty(req.Description),
		AllowedTypes:      req.AllowedTypes,
		AllowedConnectors: req.AllowedConnectors,
		AllowedMethods:    req.AllowedMethods,
		TraceInput:        true,
		TraceOutput:       true,
		RetentionDays:     30,
		Mode:              "enforce",
		Status:            "active",
		Config:            req.Config,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := h.app.CreatePolicy(ctx, policy); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	responseJSON(w, http.StatusCreated, MapPolicyToAPI(policy))
}
