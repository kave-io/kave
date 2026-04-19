package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/contract"
)

// listSpans returns all spans matching the filter.
func (h *Handler) listSpans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	runID := getQueryParam(r, "run_id")
	hasError := getQueryParamBool(r, "has_error")

	filter := &runtimemodel.SpanFilter{
		RunID:    runID,
		HasError: hasError,
	}

	page := pageQuery(r)
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

// streamSpans streams run events via SSE.
func (h *Handler) streamSpans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse optional filter params
	projectID := getQueryParam(r, "project_id")
	envID := getQueryParam(r, "env_id")
	runID := getQueryParam(r, "run_id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		errorJSON(w, http.StatusInternalServerError, "stream.unsupported", "streaming unsupported")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Subscribe to run events from the bus
	eventCh, cancel := h.bus.Subscribe()
	defer cancel()

	// Send initial comment to confirm connection
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Stream events
	for {
		select {
		case event := <-eventCh:
			// Check if event matches filters
			if projectID != "" && event.ProjectID != projectID {
				continue
			}
			if envID != "" && event.EnvID != envID {
				continue
			}
			if runID != "" && event.RunID != runID {
				continue
			}

			// Fetch full span data from database
			spans, err := h.spans.QuerySpans(ctx, &runtimemodel.SpanFilter{ID: event.SpanID}, store.Page{Limit: 1})
			if err != nil || len(spans.Items) == 0 {
				continue
			}

			// Convert to API format
			apiSpan := MapSpanRowToAPI(spans.Items[0])
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
			// Send keepalive comment
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
