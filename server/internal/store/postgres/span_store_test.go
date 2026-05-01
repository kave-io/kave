package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/store/storetest"
	"github.com/kave-io/kave/server/internal/store/postgres"
)

// TestPostgresSpanStoreSuite runs the shared SpanStore contract suite against a
// real Postgres instance. Set KAVE_TEST_POSTGRES_DSN to enable.
//
// Example:
//
//	KAVE_TEST_POSTGRES_DSN="postgres://kave:kave@localhost:5432/kave_test?sslmode=disable" go test ./internal/store/postgres/...
func TestPostgresSpanStoreSuite(t *testing.T) {
	dsn := os.Getenv("KAVE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KAVE_TEST_POSTGRES_DSN not set; skipping postgres integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	s := postgres.NewSpanStore(pool, dsn)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() {
		// Clean up spans table after each test run to prevent cross-run pollution.
		_, _ = pool.Exec(ctx, "DELETE FROM spans WHERE project_id = 'prj_test'")
	})

	storetest.RunSpanSuite(t, s)
}
