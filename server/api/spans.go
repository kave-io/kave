package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/kave-io/kave/core/store"
)

func (a *API) listSpans(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := &store.SpanFilter{
		RunID:    q.Get("run_id"),
		ActionID: q.Get("action_id"),
	}

	if s := q.Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
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
	if s := q.Get("has_error"); s != "" {
		b := s == "true"
		filter.HasError = &b
	}

	spans, err := a.span.QuerySpans(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list spans: %v", err))
		return
	}

	out := make([]spanResp, len(spans))
	for i, s := range spans {
		out[i] = toSpanResp(s)
	}
	writeJSON(w, http.StatusOK, out)
}
