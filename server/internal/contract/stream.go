package contract

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// StreamFrame is the line-delimited event envelope used by stream endpoints.
type StreamFrame struct {
	SchemaVersion string          `json:"schema_version"`
	Kind          string          `json:"kind"`
	Data          json.RawMessage `json:"data,omitempty"`
	At            int64           `json:"at_ms"`
	Reason        string          `json:"reason,omitempty"`
}

// WriteFrame writes one JSONL frame and flushes the writer when possible.
func WriteFrame(w io.Writer, f StreamFrame) error {
	if f.SchemaVersion == "" {
		f.SchemaVersion = strconv.Itoa(SchemaVersion)
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(raw, '\n')); err != nil {
		return err
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}
