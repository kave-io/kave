package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kave-io/kave/core/store"
)

func init() {
	// RegisterRoutes adds this via the API struct, but we declare the route here
	// so the file is self-contained. See api.go for route registration.
}

// watchSpans streams new spans as Server-Sent Events.
// Query params: run_id (optional), action_id (optional).
// Polls the SpanStore every 500ms and pushes new spans to connected clients.
//
// GET /api/v1/spans/stream
func (a *API) watchSpans(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	q := r.URL.Query()
	filter := &store.SpanFilter{
		RunID:    q.Get("run_id"),
		ActionID: q.Get("action_id"),
		Limit:    50, // max per poll
	}

	// Start cursor at now so we only stream spans created after connecting
	cursor := time.Now().UnixMilli()

	// Send a heartbeat immediately so the client knows the connection is alive
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fromMs := cursor
			filter.FromMs = &fromMs

			spans, err := a.span.QuerySpans(r.Context(), filter)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
				flusher.Flush()
				return
			}

			for _, span := range spans {
				data, err := json.Marshal(toSpanResp(span))
				if err != nil {
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", data)

				// Advance cursor past this span
				if span.CreatedAt > cursor {
					cursor = span.CreatedAt
				}
			}

			if len(spans) > 0 {
				flusher.Flush()
			}
		}
	}
}
