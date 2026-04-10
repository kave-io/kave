package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kave-io/kave/core/store"
)

type workspaceResp struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type createWorkspaceReq struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

func toWorkspaceResp(w *store.Workspace) workspaceResp {
	return workspaceResp{
		ID:          w.ID,
		Name:        w.Name,
		Slug:        w.Slug,
		Description: w.Description,
		CreatedAt:   w.CreatedAt,
		UpdatedAt:   w.UpdatedAt,
	}
}

func (a *API) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var req createWorkspaceReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Slug == "" {
		req.Slug = req.Name
	}

	now := time.Now().UnixMilli()
	ws := &store.Workspace{
		ID:          uuid.NewString(),
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := a.app.CreateWorkspace(r.Context(), ws); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create workspace: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, toWorkspaceResp(ws))
}

func (a *API) getWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ws, err := a.app.GetWorkspace(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("get workspace: %v", err))
		return
	}
	if ws == nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	writeJSON(w, http.StatusOK, toWorkspaceResp(ws))
}
