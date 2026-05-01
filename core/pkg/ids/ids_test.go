package ids

import (
	"strings"
	"sync"
	"testing"
)

func TestNew_prefix(t *testing.T) {
	id := New("agt")
	if !strings.HasPrefix(id, "agt_") {
		t.Fatalf("expected agt_ prefix, got %q", id)
	}
}

func TestNew_emptyPrefix(t *testing.T) {
	id := New("")
	if strings.Contains(id, "_") {
		t.Fatalf("expected bare ULID without underscore, got %q", id)
	}
	if len(id) == 0 {
		t.Fatal("expected non-empty ULID")
	}
}

func TestNew_uniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := New("run")
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate ID at iteration %d: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestNew_concurrentUniqueness(t *testing.T) {
	const workers = 8
	const each = 200
	results := make([][]string, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids := make([]string, each)
			for i := 0; i < each; i++ {
				ids[i] = New("spn")
			}
			results[w] = ids
		}()
	}
	wg.Wait()

	all := make(map[string]struct{}, workers*each)
	for _, ids := range results {
		for _, id := range ids {
			if _, dup := all[id]; dup {
				t.Fatalf("concurrent duplicate ID: %q", id)
			}
			all[id] = struct{}{}
		}
	}
}

func TestTraceID(t *testing.T) {
	id, err := TraceID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Fatalf("expected 32-char hex trace ID, got len %d: %q", len(id), id)
	}

	id2, _ := TraceID()
	if id == id2 {
		t.Fatal("two TraceIDs should not be equal")
	}
}

func TestSpanID(t *testing.T) {
	id, err := SpanID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 16 {
		t.Fatalf("expected 16-char hex span ID, got len %d: %q", len(id), id)
	}

	id2, _ := SpanID()
	if id == id2 {
		t.Fatal("two SpanIDs should not be equal")
	}
}

func TestNew_knownPrefixes(t *testing.T) {
	prefixes := []string{"act", "agt", "aud", "bge", "bnd", "cred", "env", "mbr", "org", "pat", "pol", "prj", "psn", "role", "run", "ses", "spn", "tok", "trc", "usr"}
	for _, p := range prefixes {
		id := New(p)
		if !strings.HasPrefix(id, p+"_") {
			t.Errorf("prefix %q: got %q", p, id)
		}
	}
}
