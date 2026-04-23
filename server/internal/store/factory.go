package store

import (
	"context"
	"fmt"
	"sort"
	"sync"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/config"
	postgresdb "github.com/kave-io/kave/server/internal/store/postgres"
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

// DefaultSpanStore returns the span store for the default (unkeyed) agent.
// Used by gRPC handlers that operate across all agents.
func (m *Manager) DefaultSpanStore(ctx context.Context) (store.SpanStore, error) {
	return m.SpanStore(ctx, "")
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

func (m *Manager) QuerySpans(ctx context.Context, filter *runtimemodel.SpanFilter, page store.Page) (store.PageResult[*runtimemodel.SpanRow], error) {
	stores, err := m.allSpanStores(ctx)
	if err != nil {
		return store.PageResult[*runtimemodel.SpanRow]{}, err
	}

	var spans []*runtimemodel.SpanRow
	for _, spanStore := range stores {
		result, err := spanStore.QuerySpans(ctx, filter, page)
		if err != nil {
			return store.PageResult[*runtimemodel.SpanRow]{}, err
		}
		spans = append(spans, result.Items...)
	}

	sort.Slice(spans, func(i, j int) bool {
		return spans[i].StartedAt > spans[j].StartedAt
	})
	limit := page.Limit
	if limit <= 0 {
		limit = 100
	}
	if len(spans) > limit {
		spans = spans[:limit]
	}
	return store.PageResult[*runtimemodel.SpanRow]{Items: spans}, nil
}

func (m *Manager) Migrate(ctx context.Context) error {
	_, err := m.allSpanStores(ctx)
	return err
}

func newAppStoreFromSpec(ctx context.Context, spec config.StoreSpec, postgresCfg config.PostgresConfig) (store.AppStore, error) {
	switch spec.Kind {
	case "postgres":
		dsn := storeDSN(spec, postgresCfg)
		pool, err := postgresdb.NewWithDSN(ctx, dsn, postgresCfg)
		if err != nil {
			return nil, err
		}
		return postgresimpl.New(pool, dsn), nil

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
		dsn := storeDSN(spec, postgresCfg)
		pool, err := postgresdb.NewWithDSN(ctx, dsn, postgresCfg)
		if err != nil {
			return nil, err
		}
		return postgresimpl.NewSpanStore(pool, dsn), nil

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
