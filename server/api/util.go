package api

import (
	"time"

	"github.com/kave-io/kave/core/pkg/ids"
)

// generateID generates a prefixed ULID (e.g. agn_01H...).
func generateID(prefix string) string { return ids.New(prefix) }

const apiDefaultCurrency = "USD"

// getCurrentTimeMs returns the current time in milliseconds since epoch.
func getCurrentTimeMs() int64 {
	return time.Now().UnixMilli()
}

func isoFromMS(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

func isoFromMSPtr(ms *int64) *string {
	if ms == nil {
		return nil
	}
	iso := isoFromMS(*ms)
	return &iso
}

// stringOrEmpty returns the string value or empty string if nil.
func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
