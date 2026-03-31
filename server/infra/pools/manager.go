package pools

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/alitto/pond/v2"
	"github.com/kave-io/kave/server/config"
)

// Manager creates and manages worker pools with fine-grained configuration.
// Each pool is provisioned with specific behavior: queue size, blocking mode,
// panic recovery, and whether tasks return results.
// Models are NOT loaded at startup — they wake on first task and persist for batches.
type Manager struct {
	pools  map[string]*PoolWrapper
	mu     sync.RWMutex
	config config.PoolsConfig
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a pool manager and provisions all configured pools.
// cfg.Pools contains per-pool configuration; global defaults apply to undefined pools.
func New(ctx context.Context, poolsConfig config.PoolsConfig) (*Manager, error) {
	managerCtx, cancel := context.WithCancel(ctx)

	m := &Manager{
		pools:  make(map[string]*PoolWrapper),
		config: poolsConfig,
		ctx:    managerCtx,
		cancel: cancel,
	}

	// Define all skills that should have pools provisioned
	allSkills := []string{
		// Embedding (critical path)
		"embed",
		// Inference
		"summarize", "review", "testgen", "architect", "diff", "docgen",
		"ask", "fix", "refactor", "commit", "changelog",
		"chat", "explain", "draft", "improve", "translate",
		"search", "index", "plan", "vision", "deep-review",
	}

	// Create each pool with its configuration
	for _, skill := range allSkills {
		cfg, ok := poolsConfig.Pools[skill]
		if !ok {
			// Use defaults for unconfigured skills
			cfg = m.defaultConfigFor(skill)
		}

		// Validate and apply defaults
		if cfg.MaxConcurrency <= 0 {
			cfg.MaxConcurrency = m.defaultWorkersFor(skill)
		}
		if cfg.ResultMode == "" {
			cfg.ResultMode = "fire-and-forget"
		}
		if cfg.Description == "" {
			cfg.Description = fmt.Sprintf("Worker pool for %s tasks", skill)
		}
		// PanicRecovery defaults to true (enabled)
		if cfg.QueueSize == 0 {
			// 0 means no queue (tasks rejected if workers busy)
		} else if cfg.QueueSize < 0 {
			cfg.QueueSize = 0 // Convert -1 or other negatives to unbounded via omission
		}

		// Create the pond pool with specified options
		pondPool, err := m.createPondPool(managerCtx, cfg)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("create pool %q: %w", skill, err)
		}

		wrapper := NewPoolWrapper(skill, pondPool, cfg)
		m.pools[skill] = wrapper

		slog.Info("provisioned pool", "skill", skill, "workers", cfg.MaxConcurrency,
			"queue_size", cfg.QueueSize, "non_blocking", cfg.NonBlocking,
			"panic_recovery", cfg.PanicRecovery, "result_mode", cfg.ResultMode)
	}

	return m, nil
}

// defaultConfigFor returns the base configuration for a skill.
func (m *Manager) defaultConfigFor(skill string) config.TaskPoolConfig {
	workers := m.defaultWorkersFor(skill)
	return config.TaskPoolConfig{
		MaxConcurrency: workers,
		QueueSize:      0, // Unbounded
		NonBlocking:    false,
		PanicRecovery:  true,
		ResultMode:     "fire-and-forget",
		Description:    fmt.Sprintf("Worker pool for %s tasks", skill),
	}
}

// defaultWorkersFor returns default worker count for a skill.
func (m *Manager) defaultWorkersFor(skill string) int {
	if skill == "embed" {
		if m.config.EmbedWorkers > 0 {
			return m.config.EmbedWorkers
		}
		return 8 // Network-bound: embeddings benefit from more concurrency
	}
	// All inference skills
	if m.config.InferenceWorkers > 0 {
		return m.config.InferenceWorkers
	}
	return 4 // Can be CPU-bound on some models
}

// createPondPool creates a pond pool with the specified configuration.
func (m *Manager) createPondPool(ctx context.Context, cfg config.TaskPoolConfig) (pond.Pool, error) {
	opts := []pond.Option{
		pond.WithContext(ctx),
	}

	// Queue sizing: pond.WithQueueSize(size)
	// - 0 or omitted = unbounded
	// - >0 = bounded queue with that size
	if cfg.QueueSize > 0 {
		opts = append(opts, pond.WithQueueSize(cfg.QueueSize))
	}

	// Non-blocking mode: if queue is full, reject instead of block
	if cfg.NonBlocking && cfg.QueueSize > 0 {
		opts = append(opts, pond.WithNonBlocking(true))
	}

	// Panic recovery: enabled by default, disable if requested
	if !cfg.PanicRecovery {
		opts = append(opts, pond.WithoutPanicRecovery())
	}

	// Create appropriate pool type
	if cfg.ResultMode == "result-returning" {
		// ResultPool for tasks that return values
		// Note: pond uses generics, so we'd need a generic wrapper
		// For now, use standard Pool — ResultPool requires type parameter
		return pond.NewPool(cfg.MaxConcurrency, opts...), nil
	}

	// Standard Pool for fire-and-forget tasks
	return pond.NewPool(cfg.MaxConcurrency, opts...), nil
}

// GetPool returns the pool wrapper for a given skill.
// Returns nil if pool not found.
func (m *Manager) GetPool(skill string) *PoolWrapper {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.pools[skill]
}

// GetPoolDirect returns the underlying pond.Pool for direct task submission.
func (m *Manager) GetPoolDirect(skill string) pond.Pool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if wrapper, ok := m.pools[skill]; ok {
		return wrapper.Pool()
	}
	return nil
}

// ListMetrics returns metrics for all pools.
func (m *Manager) ListMetrics() []PoolMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := make([]PoolMetrics, 0, len(m.pools))
	for _, wrapper := range m.pools {
		metrics = append(metrics, wrapper.GetMetrics())
	}
	return metrics
}

// GetMetrics returns metrics for a specific pool.
func (m *Manager) GetMetrics(skill string) *PoolMetrics {
	m.mu.RLock()
	wrapper, ok := m.pools[skill]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	metrics := wrapper.GetMetrics()
	return &metrics
}

// Resize dynamically changes a pool's concurrency.
func (m *Manager) Resize(skill string, newConcurrency int) error {
	m.mu.RLock()
	wrapper, ok := m.pools[skill]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("pool not found: %s", skill)
	}

	if newConcurrency <= 0 {
		return fmt.Errorf("concurrency must be > 0, got %d", newConcurrency)
	}

	wrapper.Resize(newConcurrency)
	slog.Info("resized pool", "skill", skill, "new_concurrency", newConcurrency)
	return nil
}

// Stop gracefully shuts down all pools.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	wrappers := make([]*PoolWrapper, 0, len(m.pools))
	for _, wrapper := range m.pools {
		wrappers = append(wrappers, wrapper)
	}
	m.mu.Unlock()

	// Cancel the manager context
	m.cancel()

	// Stop all pools (allow in-flight tasks to complete)
	for _, wrapper := range wrappers {
		wrapper.Stop()
	}

	// Wait for completion or context timeout
	done := make(chan struct{})
	go func() {
		for _, wrapper := range wrappers {
			wrapper.StopAndWait()
		}
		close(done)
	}()

	select {
	case <-done:
		slog.Info("all pools stopped gracefully")
		return nil
	case <-ctx.Done():
		slog.Warn("pool shutdown timeout, some tasks may not have completed")
		return ctx.Err()
	}
}

// ListSkills returns all provisioned pool names.
func (m *Manager) ListSkills() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	skills := make([]string, 0, len(m.pools))
	for skill := range m.pools {
		skills = append(skills, skill)
	}
	return skills
}
