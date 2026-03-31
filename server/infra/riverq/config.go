package riverq

import (
	"time"

	"github.com/riverqueue/river"
)

// RiverConfig configures River job queue behavior.
type RiverConfig struct {
	MaxWorkers       int
	QueueConcurrency map[string]int
	ShutdownTimeout  time.Duration
}

// DefaultRiverConfig returns sensible defaults for River.
func DefaultRiverConfig() RiverConfig {
	return RiverConfig{
		MaxWorkers: 10,
		QueueConcurrency: map[string]int{
			river.QueueDefault: 5, // "default"
			"embed":            2,
			"ingest":           1,
		},
		ShutdownTimeout: 30 * time.Second,
	}
}
