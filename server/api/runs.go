package api

import (
	"net/http"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
)

// listRuns returns all runs matching the filter.
func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	filter := &runtimemodel.RunFilter{
		EnvID:     getQueryParam(r, "env_id"),
		ProjectID: getQueryParam(r, "project_id"),
		AgentID:   getQueryParam(r, "agent_id"),
		Status:    getQueryParam(r, "status"),
	}

	page := pageQuery(r)
	runs, err := h.app.ListRuns(ctx, filter, page)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "store.list_failed", err.Error())
		return
	}

	result := make([]*Run, 0, len(runs.Items))
	for _, r := range runs.Items {
		result = append(result, MapRunToAPI(r))
	}

	pagedResponseJSON(w, http.StatusOK, "RunList", result, page.Limit, runs.NextCursor, nil)
}

// getRun returns a single run.
func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := getPathParam(r, "id")

	run, err := h.app.GetRunByID(ctx, id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "store.get_failed", err.Error())
		return
	}
	if run == nil {
		errorJSON(w, http.StatusNotFound, "run.not_found", "run not found")
		return
	}

	responseJSON(w, http.StatusOK, "Run", MapRunToAPI(run), nil, nil)
}

// getRunSpans returns all spans for a run.
func (h *Handler) getRunSpans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runID := getPathParam(r, "id")

	page := pageQuery(r)
	filter := &runtimemodel.SpanFilter{
		RunID: runID,
	}

	spans, err := h.spans.QuerySpans(ctx, filter, page)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "store.list_failed", err.Error())
		return
	}

	result := make([]*Span, 0, len(spans.Items))
	for _, s := range spans.Items {
		result = append(result, MapSpanRowToAPI(s))
	}

	pagedResponseJSON(w, http.StatusOK, "SpanList", result, page.Limit, spans.NextCursor, nil)
}
