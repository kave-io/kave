package bus

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSubscribeFiltersByPrefixAndFields(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe(Filter{
		Kinds:     []string{"span."},
		ProjectID: "proj-1",
		EnvID:     "env-1",
		RunID:     "run-1",
	})
	defer cancel()

	b.Publish(Event{Kind: "run.started", ProjectID: "proj-1", EnvID: "env-1", RunID: "run-1", At: 1})
	b.Publish(Event{Kind: "span.completed", ProjectID: "proj-1", EnvID: "env-1", RunID: "run-1", At: 2, Payload: json.RawMessage(`{"ok":true}`)})
	b.Publish(Event{Kind: "span.completed", ProjectID: "proj-2", EnvID: "env-1", RunID: "run-1", At: 3})

	select {
	case ev := <-ch:
		if ev.Kind != "span.completed" {
			t.Fatalf("event kind = %q, want span.completed", ev.Kind)
		}
		if ev.At != 2 {
			t.Fatalf("event at = %d, want 2", ev.At)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered event")
	}

	select {
	case ev := <-ch:
		t.Fatalf("unexpected extra event: %+v", ev)
	default:
	}
}

func TestPublishDropsSlowSubscribers(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe(Filter{})
	defer cancel()

	for i := 0; i < 128; i++ {
		b.Publish(Event{Kind: "daemon.log", At: int64(i)})
	}

	if got := len(ch); got != cap(ch) {
		t.Fatalf("buffer length = %d, want %d", got, cap(ch))
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe(Filter{})
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel still open after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}
