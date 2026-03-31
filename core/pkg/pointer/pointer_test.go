package pointer_test

import (
	"testing"

	"github.com/kave-io/kave/core/pkg/pointer"
)

func TestTo(t *testing.T) {
	p := pointer.To(42)
	if p == nil || *p != 42 {
		t.Fatalf("To(42): got %v", p)
	}
	s := pointer.To("hello")
	if *s != "hello" {
		t.Fatalf("To(%q): got %v", "hello", s)
	}
}

func TestFrom(t *testing.T) {
	v := 7
	if got := pointer.From(&v, 0); got != 7 {
		t.Errorf("From non-nil: got %d, want 7", got)
	}
	if got := pointer.From[int](nil, 99); got != 99 {
		t.Errorf("From nil: got %d, want 99", got)
	}
}

func TestFromZero(t *testing.T) {
	if got := pointer.FromZero[int](nil); got != 0 {
		t.Errorf("FromZero nil int: got %d", got)
	}
	s := "x"
	if got := pointer.FromZero(&s); got != "x" {
		t.Errorf("FromZero &s: got %q", got)
	}
}

func TestCoalesce(t *testing.T) {
	a := "first"
	b := "second"
	if got := pointer.Coalesce[string](nil, nil, &a, &b); got != "first" {
		t.Errorf("Coalesce: got %q, want %q", got, "first")
	}
	if got := pointer.Coalesce[string](nil, nil); got != "" {
		t.Errorf("Coalesce all-nil: got %q", got)
	}
}

func TestMap(t *testing.T) {
	n := 3
	doubled := pointer.Map(&n, func(v int) int { return v * 2 })
	if doubled == nil || *doubled != 6 {
		t.Errorf("Map non-nil: got %v", doubled)
	}
	if got := pointer.Map[int, int](nil, func(v int) int { return v }); got != nil {
		t.Errorf("Map nil: expected nil, got %v", got)
	}
}

func TestIf(t *testing.T) {
	if p := pointer.If(true, "yes"); p == nil || *p != "yes" {
		t.Errorf("If true: got %v", p)
	}
	if p := pointer.If(false, "no"); p != nil {
		t.Errorf("If false: expected nil, got %v", p)
	}
}

func TestEqual(t *testing.T) {
	a, b := 5, 5
	c := 6
	if !pointer.Equal(&a, &b) {
		t.Error("Equal same value: expected true")
	}
	if pointer.Equal(&a, &c) {
		t.Error("Equal different value: expected false")
	}
	if !pointer.Equal[int](nil, nil) {
		t.Error("Equal nil,nil: expected true")
	}
	if pointer.Equal(&a, nil) {
		t.Error("Equal non-nil,nil: expected false")
	}
}
