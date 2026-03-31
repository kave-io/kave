package riverq

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// New creates a River client configured for durable job processing.
// Caller is responsible for calling client.Start(ctx) and client.Stop(ctx).
func New(pool *pgxpool.Pool, workers *river.Workers, cfg RiverConfig) (*river.Client[pgx.Tx], error) {
	// Build queues map from config
	queues := make(map[string]river.QueueConfig, len(cfg.QueueConcurrency))
	for name, concurrency := range cfg.QueueConcurrency {
		queues[name] = river.QueueConfig{MaxWorkers: concurrency}
	}

	// Create and return River client
	riverClient, err := river.NewClient(
		riverpgxv5.New(pool),
		&river.Config{
			Workers: workers,
			Queues:  queues,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("river client: %w", err)
	}

	return riverClient, nil
}
