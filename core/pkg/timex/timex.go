// Package timex provides a named millisecond timestamp type to make UnixMilli
// usage explicit and prevent unit confusion (seconds vs milliseconds).
package timex

import "time"

// MS is a Unix timestamp in milliseconds.
// Zero value means "not set" — Jan 1 1970 is never a valid action timestamp.
type MS int64

// Now returns the current time as MS.
func Now() MS {
	return MS(time.Now().UnixMilli())
}

// From converts a time.Time to MS.
func From(t time.Time) MS {
	return MS(t.UnixMilli())
}

// Time converts MS to time.Time in UTC.
func (m MS) Time() time.Time {
	return time.UnixMilli(int64(m)).UTC()
}

// IsZero reports whether m is unset.
func (m MS) IsZero() bool {
	return m == 0
}

// String formats as RFC3339. Returns empty string for zero value.
func (m MS) String() string {
	if m.IsZero() {
		return ""
	}
	return m.Time().Format(time.RFC3339Nano)
}

// Since returns the elapsed milliseconds since m.
func Since(m MS) int64 {
	return int64(Now() - m)
}
