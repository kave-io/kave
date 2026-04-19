// Package bus provides a lightweight in-process pub/sub for typed events.
// Slow subscribers are dropped (non-blocking send) to protect publishers.
package bus

import (
	"encoding/json"
	"strings"
	"sync"
)

// Event is the typed envelope published across the in-process bus.
type Event struct {
	Kind      string          `json:"kind"`
	ProjectID string          `json:"project_id,omitempty"`
	EnvID     string          `json:"env_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	AgentID   string          `json:"agent_id,omitempty"`
	SpanID    string          `json:"span_id,omitempty"`
	At        int64           `json:"at_ms"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// Filter narrows the events delivered to one subscriber.
type Filter struct {
	Kinds     []string
	ProjectID string
	EnvID     string
	RunID     string
}

type subscriber struct {
	filter Filter
	ch     chan Event
	once   sync.Once
}

// Bus fans out typed events to active subscribers.
// Safe for concurrent use.
type Bus struct {
	mu   sync.RWMutex
	subs map[chan Event]*subscriber
}

// New returns a ready-to-use Bus.
func New() *Bus {
	return &Bus{subs: make(map[chan Event]*subscriber)}
}

// Publish sends ev to all matching subscribers. Slow subscribers are dropped.
func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs {
		if !matches(sub.filter, ev) {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
		}
	}
}

// Subscribe returns a buffered channel that receives matching Events and a
// cancel function the caller must invoke when done.
func (b *Bus) Subscribe(filter Filter) (<-chan Event, func()) {
	ch := make(chan Event, 64)
	sub := &subscriber{filter: filter, ch: ch}

	b.mu.Lock()
	b.subs[ch] = sub
	b.mu.Unlock()

	cancel := func() {
		sub.once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// SubscriberCount returns the number of active subscribers.
func (b *Bus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

func matches(filter Filter, ev Event) bool {
	if filter.ProjectID != "" && filter.ProjectID != ev.ProjectID {
		return false
	}
	if filter.EnvID != "" && filter.EnvID != ev.EnvID {
		return false
	}
	if filter.RunID != "" && filter.RunID != ev.RunID {
		return false
	}
	if len(filter.Kinds) == 0 {
		return true
	}
	for _, kind := range filter.Kinds {
		if kind == "" {
			continue
		}
		if strings.HasPrefix(ev.Kind, kind) {
			return true
		}
	}
	return false
}
