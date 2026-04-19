package httpbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kave-io/kave/core/bus"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/contract"
)

const streamHeartbeatInterval = 15 * time.Second

// RegisterStreams installs bridge-managed streaming endpoints.
func RegisterStreams(mux *http.ServeMux, app store.AppStore, spans store.SpanStore, b *bus.Bus) {
	mux.HandleFunc("GET /api/v1/trace/tail", traceTailHandler(app, spans, b))
	mux.HandleFunc("GET /api/v1/events/tail", eventsTailHandler(b))
	mux.HandleFunc("GET /api/v1/logs/tail", logsTailHandler(b))
	mux.HandleFunc("GET /api/v1/spans/stream", spansStreamHandler(spans, b))
}

func traceTailHandler(app store.AppStore, spans store.SpanStore, b *bus.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !prepareJSONLStream(w) {
			return
		}
		ch, cancel := b.Subscribe(bus.Filter{
			Kinds:     []string{"run.", "span."},
			ProjectID: r.URL.Query().Get("project_id"),
			EnvID:     r.URL.Query().Get("env_id"),
			RunID:     r.URL.Query().Get("run_id"),
		})
		defer cancel()

		streamLoop(r.Context(), w, ch, func(ev bus.Event) (contract.StreamFrame, bool, error) {
			switch {
			case strings.HasPrefix(ev.Kind, "run."):
				run, err := app.GetRunByID(r.Context(), ev.RunID)
				if err != nil || run == nil {
					return contract.StreamFrame{}, false, nil
				}
				raw, err := json.Marshal(MapRunToAPI(run))
				if err != nil {
					return contract.StreamFrame{}, false, nil
				}
				return contract.StreamFrame{
					Kind: "Run",
					At:   ev.At,
					Data: raw,
				}, true, nil
			case ev.Kind == "span.completed":
				row, err := spans.GetSpan(r.Context(), ev.SpanID)
				if err != nil || row == nil {
					return contract.StreamFrame{}, false, nil
				}
				raw, err := json.Marshal(MapSpanRowToAPI(row))
				if err != nil {
					return contract.StreamFrame{}, false, nil
				}
				return contract.StreamFrame{
					Kind: "Span",
					At:   ev.At,
					Data: raw,
				}, true, nil
			default:
				return contract.StreamFrame{}, false, nil
			}
		}, writeJSONLFrame)
	}
}

func eventsTailHandler(b *bus.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !prepareJSONLStream(w) {
			return
		}
		filter := bus.Filter{
			ProjectID: r.URL.Query().Get("project_id"),
			EnvID:     r.URL.Query().Get("env_id"),
		}
		if kind := r.URL.Query().Get("kind"); kind != "" {
			filter.Kinds = []string{kind}
		}
		ch, cancel := b.Subscribe(filter)
		defer cancel()

		streamLoop(r.Context(), w, ch, func(ev bus.Event) (contract.StreamFrame, bool, error) {
			raw := ev.Payload
			if len(raw) == 0 {
				raw = json.RawMessage(`{}`)
			}
			return contract.StreamFrame{
				Kind: "Event",
				At:   ev.At,
				Data: raw,
			}, true, nil
		}, writeJSONLFrame)
	}
}

func logsTailHandler(b *bus.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !prepareJSONLStream(w) {
			return
		}
		ch, cancel := b.Subscribe(bus.Filter{Kinds: []string{"daemon.log"}})
		defer cancel()

		wantLevel := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("level")))
		streamLoop(r.Context(), w, ch, func(ev bus.Event) (contract.StreamFrame, bool, error) {
			var payload struct {
				Level string `json:"level"`
			}
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				return contract.StreamFrame{}, false, nil
			}
			if wantLevel != "" && strings.ToLower(payload.Level) != wantLevel {
				return contract.StreamFrame{}, false, nil
			}
			raw := ev.Payload
			if len(raw) == 0 {
				raw = json.RawMessage(`{}`)
			}
			return contract.StreamFrame{
				Kind: "LogLine",
				At:   ev.At,
				Data: raw,
			}, true, nil
		}, writeJSONLFrame)
	}
}

func spansStreamHandler(spans store.SpanStore, b *bus.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "stream.unsupported", "streaming unsupported", nil)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch, cancel := b.Subscribe(bus.Filter{
			Kinds:     []string{"span.completed"},
			ProjectID: r.URL.Query().Get("project_id"),
			EnvID:     r.URL.Query().Get("env_id"),
			RunID:     r.URL.Query().Get("run_id"),
		})
		defer cancel()

		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		streamLoop(ctx, w, ch, func(ev bus.Event) (contract.StreamFrame, bool, error) {
			row, err := spans.GetSpan(ctx, ev.SpanID)
			if err != nil || row == nil {
				return contract.StreamFrame{}, false, nil
			}
			raw, err := json.Marshal(contract.SuccessEnvelope{
				SchemaVersion: contract.SchemaVersion,
				Kind:          "Span",
				Data:          MapSpanRowToAPI(row),
				Page:          nil,
				Warnings:      []contract.Warning{},
			})
			if err != nil {
				return contract.StreamFrame{}, false, nil
			}
			return contract.StreamFrame{
				Kind: "Span",
				At:   ev.At,
				Data: raw,
			}, true, nil
		}, writeSSEFrame)
	}
}

type streamRenderer func(ev bus.Event) (contract.StreamFrame, bool, error)

type streamEmitter func(http.ResponseWriter, contract.StreamFrame) error

func streamLoop(ctx context.Context, w http.ResponseWriter, ch <-chan bus.Event, render streamRenderer, emit streamEmitter) {
	ticker := time.NewTicker(streamHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = emit(w, contract.StreamFrame{
				SchemaVersion: schemaVersionString(),
				Kind:          "StreamClosed",
				At:            time.Now().UnixMilli(),
				Reason:        "context canceled",
			})
			return
		case ev, ok := <-ch:
			if !ok {
				_ = emit(w, contract.StreamFrame{
					SchemaVersion: schemaVersionString(),
					Kind:          "StreamClosed",
					At:            time.Now().UnixMilli(),
					Reason:        "publisher closed",
				})
				return
			}
			frame, ok, err := render(ev)
			if err != nil || !ok {
				continue
			}
			_ = emit(w, frame)
		case <-ticker.C:
			_ = emit(w, contract.StreamFrame{
				SchemaVersion: schemaVersionString(),
				Kind:          "Heartbeat",
				At:            time.Now().UnixMilli(),
			})
		}
	}
}

func prepareJSONLStream(w http.ResponseWriter) bool {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "stream.unsupported", "streaming unsupported", nil)
		return false
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()
	return true
}

func writeJSONLFrame(w http.ResponseWriter, frame contract.StreamFrame) error {
	return contract.WriteFrame(w, frame)
}

func writeSSEFrame(w http.ResponseWriter, frame contract.StreamFrame) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil
	}
	switch frame.Kind {
	case "Heartbeat":
		fmt.Fprint(w, ": keepalive\n\n")
	case "StreamClosed":
		raw, err := json.Marshal(frame)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "event: StreamClosed\ndata: %s\n\n", raw)
	default:
		raw, err := json.Marshal(contract.SuccessEnvelope{
			SchemaVersion: contract.SchemaVersion,
			Kind:          frame.Kind,
			Data:          json.RawMessage(frame.Data),
			Page:          nil,
			Warnings:      []contract.Warning{},
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "data: %s\n\n", raw)
	}
	flusher.Flush()
	return nil
}

func schemaVersionString() string {
	return strconv.Itoa(contract.SchemaVersion)
}
