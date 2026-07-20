package postgres

import (
	"math"
	"testing"
	"time"
)

func TestAdmissionWindowFixedFloorsBeforeAnchor(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	seconds := int64(60)
	start, end, err := admissionWindow(anchor.Add(-time.Second), applicableLimit{
		key: "fixed", windowKind: "fixed", windowSeconds: &seconds, windowAnchor: &anchor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := anchor.Add(-time.Minute); !start.Equal(want) {
		t.Fatalf("start = %v, want %v", start, want)
	}
	if !end.Equal(anchor) {
		t.Fatalf("end = %v, want %v", end, anchor)
	}
}

func TestAddWouldOverflow(t *testing.T) {
	t.Parallel()

	if _, overflow := addWouldOverflow(math.MaxInt64, 1); !overflow {
		t.Fatal("expected overflow")
	}
	if got, overflow := addWouldOverflow(3, 4, 5); overflow || got != 12 {
		t.Fatalf("got=%d overflow=%v, want 12/false", got, overflow)
	}
}
