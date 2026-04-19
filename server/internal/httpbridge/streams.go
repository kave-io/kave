package httpbridge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kave-io/kave/core/bus"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/contract"
)

// RegisterStreams installs bridge-managed streaming endpoints.
func RegisterStreams(mux *http.ServeMux, spans store.SpanStore, b *bus.Bus) {
	mux.HandleFunc("GET /api/v1/spans/stream", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		projectID := r.URL.Query().Get("project_id")
		envID := r.URL.Query().Get("env_id")
		runID := r.URL.Query().Get("run_id")

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "stream.unsupported", "streaming unsupported", nil)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		eventCh, cancel := b.Subscribe()
		defer cancel()

		fmt.Fprintf(w, ": connected\n\n")
		flusher.Flush()

		for {
			select {
			case event := <-eventCh:
				if projectID != "" && event.ProjectID != projectID {
					continue
				}
				if envID != "" && event.EnvID != envID {
					continue
				}
				if runID != "" && event.RunID != runID {
					continue
				}

				rows, err := spans.QuerySpans(ctx, &runtimemodel.SpanFilter{ID: event.SpanID}, store.Page{Limit: 1})
				if err != nil || len(rows.Items) == 0 {
					continue
				}

				apiSpan := MapSpanRowToAPI(rows.Items[0])
				data, err := json.Marshal(apiSpan)
				if err != nil {
					continue
				}
				envelope, err := json.Marshal(contract.SuccessEnvelope{
					SchemaVersion: contract.SchemaVersion,
					Kind:          "Span",
					Data:          json.RawMessage(data),
					Page:          nil,
					Warnings:      []contract.Warning{},
				})
				if err != nil {
					continue
				}

				fmt.Fprintf(w, "data: %s\n\n", string(envelope))
				flusher.Flush()

			case <-ctx.Done():
				return

			case <-time.After(30 * time.Second):
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	})
}
