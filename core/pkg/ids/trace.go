package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// TraceID returns a 128-bit OTel-style trace ID as lowercase hex.
func TraceID() (string, error) {
	return randomHex(16)
}

// SpanID returns a 64-bit OTel-style span ID as lowercase hex.
func SpanID() (string, error) {
	return randomHex(8)
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("ids: random read: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
