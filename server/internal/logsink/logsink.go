package logsink

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/kave-io/kave/core/bus"
)

// Sink tees standard log output to a writer and the event bus.
type Sink struct {
	dst io.Writer
	bus *bus.Bus

	mu  sync.Mutex
	buf []byte
}

// New returns a writer that mirrors log output to dst and publishes daemon.log events.
func New(dst io.Writer, b *bus.Bus) *Sink {
	return &Sink{dst: dst, bus: b}
}

// Write mirrors the bytes to dst and publishes complete log lines as events.
func (s *Sink) Write(p []byte) (int, error) {
	if s.dst != nil {
		if _, err := s.dst.Write(p); err != nil {
			return 0, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf = append(s.buf, p...)
	for {
		idx := bytes.IndexByte(s.buf, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(string(s.buf[:idx]), "\r")
		s.buf = append([]byte(nil), s.buf[idx+1:]...)
		s.publishLine(line)
	}
	return len(p), nil
}

func (s *Sink) publishLine(line string) {
	if s.bus == nil || strings.TrimSpace(line) == "" {
		return
	}
	level := logLevel(line)
	payload, err := json.Marshal(map[string]string{
		"level":   level,
		"message": line,
	})
	if err != nil {
		return
	}
	s.bus.Publish(bus.Event{
		Kind:    "daemon.log",
		At:      time.Now().UnixMilli(),
		Payload: payload,
	})
}

func logLevel(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error:"):
		return "error"
	case strings.Contains(lower, "warn:"):
		return "warn"
	case strings.Contains(lower, "debug:"):
		return "debug"
	default:
		return "info"
	}
}
