// Package bus provides a lightweight in-process pub/sub for run events.
// One goroutine publishes; N gRPC stream handlers subscribe.
// Slow subscribers are dropped (non-blocking send) to protect the publisher.
package bus

import "sync"

// RunEvent is published after every intercepted action completes.
// It carries a snapshot of the current run state at that point in time.
type RunEvent struct {
	RunID     string
	ProjectID string
	EnvID     string
	AgentID   string
	Status    string
	SpanID    string
}

// Bus fans out RunEvents to all active subscribers.
// Safe for concurrent use. Subscribe/Unsubscribe are O(n) on subscriber count,
// which is fine — number of live watchers is always tiny.
type Bus struct {
	mu   sync.RWMutex
	subs map[chan RunEvent]struct{}
}

// New returns a ready-to-use Bus.
func New() *Bus {
	return &Bus{subs: make(map[chan RunEvent]struct{})}
}

// Publish sends ev to all subscribers. Drops slow subscribers (non-blocking).
func (b *Bus) Publish(ev RunEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // subscriber too slow — drop rather than block the publisher
		}
	}
}

// Subscribe returns a buffered channel that receives RunEvents and a cancel
// function the caller must invoke when done.
func (b *Bus) Subscribe() (<-chan RunEvent, func()) {
	ch := make(chan RunEvent, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}
