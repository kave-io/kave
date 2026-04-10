package store

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/config"
	postgresdb "github.com/kave-io/kave/server/internal/db/postgres"
	duckdbimpl "github.com/kave-io/kave/server/internal/store/duckdb"
	postgresimpl "github.com/kave-io/kave/server/internal/store/postgres"
	sqliteimpl "github.com/kave-io/kave/server/internal/store/sqlite"
)

type Manager struct {
	app       store.AppStore
	storage   config.StorageConfig
	postgres  config.PostgresConfig
	spanMu    sync.Mutex
	spanCache map[string]store.SpanStore
}

func NewManager(ctx context.Context, storageCfg config.StorageConfig, postgresCfg config.PostgresConfig) (*Manager, error) {
	app, err := newAppStoreFromSpec(ctx, storageCfg.AppDefault(), postgresCfg)
	if err != nil {
		return nil, err
	}

	return &Manager{
		app:       app,
		storage:   storageCfg,
		postgres:  postgresCfg,
		spanCache: map[string]store.SpanStore{},
	}, nil
}

func (m *Manager) AppStore() store.AppStore {
	return m.app
}

func (m *Manager) SpanStore(ctx context.Context, agentID string) (store.SpanStore, error) {
	spec := m.storage.SpanForAgent(agentID)
	key := spanStoreKey(spec)

	m.spanMu.Lock()
	defer m.spanMu.Unlock()

	if span, ok := m.spanCache[key]; ok {
		return span, nil
	}

	span, err := newSpanStoreFromSpec(ctx, spec, m.postgres)
	if err != nil {
		return nil, err
	}
	if err := span.Migrate(ctx); err != nil {
		_ = span.Close()
		return nil, err
	}
	m.spanCache[key] = span
	return span, nil
}

func (m *Manager) Close() error {
	m.spanMu.Lock()
	defer m.spanMu.Unlock()

	for key, span := range m.spanCache {
		_ = span.Close()
		delete(m.spanCache, key)
	}
	if m.app != nil {
		return m.app.Close()
	}
	return nil
}

func (m *Manager) WriteSpan(ctx context.Context, span *store.SpanRow) error {
	defaultStore, err := m.SpanStore(ctx, "")
	if err != nil {
		return err
	}
	return defaultStore.WriteSpan(ctx, span)
}

func (m *Manager) UpdateSpan(ctx context.Context, span *store.SpanRow) error {
	defaultStore, err := m.SpanStore(ctx, "")
	if err != nil {
		return err
	}
	return defaultStore.UpdateSpan(ctx, span)
}

func (m *Manager) GetSpan(ctx context.Context, spanID string) (*store.SpanRow, error) {
	stores, err := m.allSpanStores(ctx)
	if err != nil {
		return nil, err
	}
	for _, spanStore := range stores {
		span, err := spanStore.GetSpan(ctx, spanID)
		if err != nil {
			return nil, err
		}
		if span != nil {
			return span, nil
		}
	}
	return nil, nil
}

func (m *Manager) QuerySpans(ctx context.Context, filter *store.SpanFilter) ([]*store.SpanRow, error) {
	stores, err := m.allSpanStores(ctx)
	if err != nil {
		return nil, err
	}

	var spans []*store.SpanRow
	for _, spanStore := range stores {
		rows, err := spanStore.QuerySpans(ctx, filter)
		if err != nil {
			return nil, err
		}
		spans = append(spans, rows...)
	}

	sort.Slice(spans, func(i, j int) bool {
		return spans[i].StartedAt > spans[j].StartedAt
	})
	if filter.Limit > 0 && len(spans) > filter.Limit {
		spans = spans[:filter.Limit]
	}
	return spans, nil
}

func (m *Manager) SpendByDimension(ctx context.Context, groupBy string, filter *store.SpanFilter) (map[string]float64, error) {
	stores, err := m.allSpanStores(ctx)
	if err != nil {
		return nil, err
	}

	total := map[string]float64{}
	for _, spanStore := range stores {
		values, err := spanStore.SpendByDimension(ctx, groupBy, filter)
		if err != nil {
			return nil, err
		}
		for key, value := range values {
			total[key] += value
		}
	}
	return total, nil
}

func (m *Manager) Migrate(ctx context.Context) error {
	_, err := m.allSpanStores(ctx)
	return err
}

func (m *Manager) allSpanStores(ctx context.Context) ([]store.SpanStore, error) {
	keys := map[string]bool{"": true}
	for agentID := range m.storage.Agents {
		keys[agentID] = true
	}

	stores := make([]store.SpanStore, 0, len(keys))
	seen := map[string]bool{}
	for agentID := range keys {
		spec := m.storage.SpanForAgent(agentID)
		key := spanStoreKey(spec)
		if seen[key] {
			continue
		}
		seen[key] = true
		spanStore, err := m.SpanStore(ctx, agentID)
		if err != nil {
			return nil, err
		}
		stores = append(stores, spanStore)
	}
	return stores, nil
}

func newAppStoreFromSpec(ctx context.Context, spec config.StoreSpec, postgresCfg config.PostgresConfig) (store.AppStore, error) {
	switch spec.Kind {
	case "postgres":
		pool, err := postgresdb.NewWithDSN(ctx, storeDSN(spec, postgresCfg), postgresCfg)
		if err != nil {
			return nil, err
		}
		return postgresimpl.New(pool), nil

	case "sqlite", "":
		path := spec.Path
		if path == "" {
			path = "kave.db"
		}
		return sqliteimpl.New(path)

	default:
		return nil, fmt.Errorf("unknown app store backend %q", spec.Kind)
	}
}

func newSpanStoreFromSpec(ctx context.Context, spec config.StoreSpec, postgresCfg config.PostgresConfig) (store.SpanStore, error) {
	switch spec.Kind {
	case "postgres":
		pool, err := postgresdb.NewWithDSN(ctx, storeDSN(spec, postgresCfg), postgresCfg)
		if err != nil {
			return nil, err
		}
		return postgresimpl.NewSpanStore(pool), nil

	case "duckdb", "":
		path := spec.Path
		if path == "" {
			path = "kave-spans.duckdb"
		}
		return duckdbimpl.New(path)

	case "clickhouse":
		return nil, fmt.Errorf("clickhouse span store is not implemented yet")

	default:
		return nil, fmt.Errorf("unknown span store backend %q", spec.Kind)
	}
}

func spanStoreKey(spec config.StoreSpec) string {
	return spec.Kind + "|" + spec.Path + "|" + spec.DSN
}

func storeDSN(spec config.StoreSpec, postgresCfg config.PostgresConfig) string {
	if spec.DSN != "" {
		return spec.DSN
	}
	return postgresCfg.UnixSocketDSN()
}
