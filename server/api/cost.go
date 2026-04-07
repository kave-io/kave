package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/kave-io/kave/core/store"
)

func (a *API) costSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := &store.SpendFilter{
		AgentID:   q.Get("agent_id"),
		Connector: q.Get("connector"),
		Model:     q.Get("model"),
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

	report, err := a.app.GetSpendReport(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("get spend report: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, toSpendResp(report))
}
