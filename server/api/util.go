package api

import (
	"time"

	"github.com/google/uuid"
)

// generateID generates a new UUID-based ID.
func generateID() string {
	return uuid.New().String()
}

// getCurrentTimeMs returns the current time in milliseconds since epoch.
func getCurrentTimeMs() int64 {
	return time.Now().UnixMilli()
}

// stringOrEmpty returns the string value or empty string if nil.
func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
