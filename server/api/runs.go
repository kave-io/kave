package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/kave-io/kave/core/store"
)

func (a *API) listRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := &store.RunFilter{
		WorkspaceID: q.Get("workspace_id"),
		AgentID:     q.Get("agent_id"),
		Status:      q.Get("status"),
	}

	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err == nil && n > 0 {
			filter.Limit = n
		}
	}
	if s := q.Get("from"); s != "" {
		if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
			filter.FromMs = &ms
		}
	}
	if s := q.Get("to"); s != "" {
		if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
			filter.ToMs = &ms
		}
	}

	runs, err := a.app.ListRuns(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list runs: %v", err))
		return
	}

	out := make([]runResp, len(runs))
	for i, ru := range runs {
		out[i] = toRunResp(ru)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) getRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := a.app.GetRunByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("get run: %v", err))
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, toRunResp(run))
}

func (a *API) getRunSpans(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	q := r.URL.Query()

	filter := &store.SpanFilter{RunID: id}

	if s := q.Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			filter.Limit = n
		}
	}

	spans, err := a.span.QuerySpans(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("get spans: %v", err))
		return
	}

	out := make([]spanResp, len(spans))
	for i, s := range spans {
		out[i] = toSpanResp(s)
	}
	writeJSON(w, http.StatusOK, out)
}
