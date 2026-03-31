package pools

import (
	"sync"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/kave-io/kave/server/config"
)

// PoolMetrics holds real-time metrics for a pond pool.
type PoolMetrics struct {
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	RunningWorkers    int64     `json:"running_workers"`
	SubmittedTasks    uint64    `json:"submitted_tasks"`
	WaitingTasks      uint64    `json:"waiting_tasks"`
	SuccessfulTasks   uint64    `json:"successful_tasks"`
	FailedTasks       uint64    `json:"failed_tasks"`
	CanceledTasks     uint64    `json:"canceled_tasks"`
	CompletedTasks    uint64    `json:"completed_tasks"`
	DroppedTasks      uint64    `json:"dropped_tasks"`
	MaxConcurrency    int       `json:"max_concurrency"`
	QueueSize         int       `json:"queue_size"`
	NonBlocking       bool      `json:"non_blocking"`
	PanicRecovery     bool      `json:"panic_recovery"`
	ResultMode        string    `json:"result_mode"`
	CreatedAt         time.Time `json:"created_at"`
	LastMetricsReadAt time.Time `json:"last_metrics_read_at"`
}

// PoolWrapper wraps a pond pool with metadata, config, and metrics.
type PoolWrapper struct {
	name    string
	pool    pond.Pool
	config  config.TaskPoolConfig
	mu      sync.RWMutex
	metrics PoolMetrics
	created time.Time
}

// NewPoolWrapper creates a new wrapper around a pond pool.
func NewPoolWrapper(name string, pool pond.Pool, cfg config.TaskPoolConfig) *PoolWrapper {
	return &PoolWrapper{
		name:   name,
		pool:   pool,
		config: cfg,
		metrics: PoolMetrics{
			Name:           name,
			Description:    cfg.Description,
			MaxConcurrency: cfg.MaxConcurrency,
			QueueSize:      cfg.QueueSize,
			NonBlocking:    cfg.NonBlocking,
			PanicRecovery:  cfg.PanicRecovery,
			ResultMode:     cfg.ResultMode,
			CreatedAt:      time.Now(),
		},
		created: time.Now(),
	}
}

// GetMetrics returns a snapshot of current pool metrics.
func (pw *PoolWrapper) GetMetrics() PoolMetrics {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	metrics := pw.metrics
	metrics.RunningWorkers = pw.pool.RunningWorkers()
	metrics.SubmittedTasks = pw.pool.SubmittedTasks()
	metrics.WaitingTasks = pw.pool.WaitingTasks()
	metrics.SuccessfulTasks = pw.pool.SuccessfulTasks()
	metrics.FailedTasks = pw.pool.FailedTasks()
	metrics.CanceledTasks = pw.pool.CanceledTasks()
	metrics.CompletedTasks = pw.pool.CompletedTasks()
	metrics.DroppedTasks = pw.pool.DroppedTasks()
	metrics.LastMetricsReadAt = time.Now()

	return metrics
}

// Pool returns the underlying pond.Pool.
func (pw *PoolWrapper) Pool() pond.Pool {
	return pw.pool
}

// Name returns the pool's name.
func (pw *PoolWrapper) Name() string {
	return pw.name
}

// Config returns the pool's configuration.
func (pw *PoolWrapper) Config() config.TaskPoolConfig {
	return pw.config
}

// Uptime returns how long the pool has been running.
func (pw *PoolWrapper) Uptime() time.Duration {
	return time.Since(pw.created)
}

// Stop gracefully shuts down the pool.
func (pw *PoolWrapper) Stop() {
	pw.pool.Stop()
}

// StopAndWait stops the pool and waits for it to finish.
func (pw *PoolWrapper) StopAndWait() {
	pw.pool.StopAndWait()
}

// Resize dynamically changes the pool's concurrency.
func (pw *PoolWrapper) Resize(newConcurrency int) {
	pw.pool.Resize(newConcurrency)
	pw.mu.Lock()
	pw.config.MaxConcurrency = newConcurrency
	pw.metrics.MaxConcurrency = newConcurrency
	pw.mu.Unlock()
}
