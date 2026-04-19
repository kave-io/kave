package contract

import (
	"encoding/json"
	"net/http"
)

// WriteSuccess writes a SuccessEnvelope as JSON.
func WriteSuccess(w http.ResponseWriter, status int, kind string, data any, page *Page, warnings []Warning) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(SuccessEnvelope{
		SchemaVersion: SchemaVersion,
		Kind:          kind,
		Data:          data,
		Page:          page,
		Warnings:      EnsureWarnings(warnings),
	})
}

// WriteError writes an ErrorEnvelope as JSON.
func WriteError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	if code == "" {
		code = "unknown"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		SchemaVersion: SchemaVersion,
		Kind:          "Error",
		Error: ErrorPayload{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

// EnsureWarnings guarantees warnings is an array (never null).
func EnsureWarnings(in []Warning) []Warning {
	if in == nil {
		return []Warning{}
	}
	return in
}
