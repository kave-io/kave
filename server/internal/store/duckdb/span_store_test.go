package duckdb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kave-io/kave/core/store/storetest"
)

func TestDuckDBSpanStoreSuite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spans.db")
	s, err := New(path)
	if err != nil {
		t.Fatalf("duckdb.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	storetest.RunSpanSuite(t, s)
}
