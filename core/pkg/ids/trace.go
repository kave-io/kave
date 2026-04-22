package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// TraceID returns a 128-bit OTel-style trace ID as lowercase hex.
func TraceID() string {
	return randomHex(16)
}

// SpanID returns a 64-bit OTel-style span ID as lowercase hex.
func SpanID() string {
	return randomHex(8)
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Errorf("ids: random read: %w", err))
	}
	return hex.EncodeToString(buf)
}
