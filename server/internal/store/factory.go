package store

import (
	"context"
	"fmt"
	"sort"
	"sync"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/config"
	clickhouseimpl "github.com/kave-io/kave/server/internal/store/clickhouse"
	duckdbimpl "github.com/kave-io/kave/server/internal/store/duckdb"
	postgresdb "github.com/kave-io/kave/server/internal/store/postgres"
	postgresimpl "github.com/kave-io/kave/server/internal/store/postgres"
	sqliteimpl "github.com/kave-io/kave/server/internal/store/sqlite"
)

type Manager struct {
	app       store.AppStore
	audit     store.AuditStore
	storage   config.StorageConfig
	postgres  config.PostgresConfig
	spanMu    sync.Mutex
	spanCache map[string]store.SpanStore
}

var _ store.SpanStore = (*Manager)(nil)

func NewManager(ctx context.Context, storageCfg config.StorageConfig, postgresCfg config.PostgresConfig) (*Manager, error) {
	app, err := newAppStoreFromSpec(ctx, storageCfg.AppDefault(), postgresCfg)
	if err != nil {
		return nil, err
	}

	audit, err := newAuditStoreFromSpec(ctx, storageCfg.AppDefault(), postgresCfg)
	if err != nil {
		return nil, err
	}

	return &Manager{
		app:       app,
		audit:     audit,
		storage:   storageCfg,
		postgres:  postgresCfg,
		spanCache: map[string]store.SpanStore{},
	}, nil
}

func (m *Manager) AppStore() store.AppStore {
	return m.app
}

func (m *Manager) AuditStore() store.AuditStore {
	return m.audit
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
	if m.audit != nil {
		_ = m.audit.Close()
	}
	if m.app != nil {
		return m.app.Close()
	}
	return nil
}

func (m *Manager) OpenSpan(ctx context.Context, span *runtimemodel.SpanRow) error {
	agentID := ""
	if span != nil {
		agentID = span.AgentID
	}
	spanStore, err := m.SpanStore(ctx, agentID)
	if err != nil {
		return err
	}
	return spanStore.OpenSpan(ctx, span)
}

func (m *Manager) CloseSpan(ctx context.Context, spanID string, end *runtimemodel.SpanEnd) error {
	row, err := m.GetSpan(ctx, spanID)
	if err != nil {
		return err
	}
	agentID := ""
	if row != nil {
		agentID = row.AgentID
	}
	spanStore, err := m.SpanStore(ctx, agentID)
	if err != nil {
		return err
	}
	return spanStore.CloseSpan(ctx, spanID, end)
}

func (m *Manager) GetSpan(ctx context.Context, spanID string) (*runtimemodel.SpanRow, error) {
	stores, err := m.allSpanStores(ctx)
	if err != nil {
		return nil, err
	}
	for _, spanStore := range stores {
		row, err := spanStore.GetSpan(ctx, spanID)
		if err != nil {
			return nil, err
		}
		if row != nil {
			return row, nil
		}
	}
	return nil, nil
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

func (m *Manager) SpendByDimension(ctx context.Context, groupBy string, filter *runtimemodel.SpanFilter) (map[string]money.Amount, error) {
	stores, err := m.allSpanStores(ctx)
	if err != nil {
		return nil, err
	}
	result := map[string]money.Amount{}
	for _, spanStore := range stores {
		partial, err := spanStore.SpendByDimension(ctx, groupBy, filter)
		if err != nil {
			return nil, err
		}
		for dimension, amount := range partial {
			next, err := result[dimension].Add(amount)
			if err != nil {
				return nil, err
			}
			result[dimension] = next
		}
	}
	return result, nil
}

func (m *Manager) Migrate(ctx context.Context) error {
	_, err := m.allSpanStores(ctx)
	return err
}

func (m *Manager) Ping(ctx context.Context) error {
	stores, err := m.allSpanStores(ctx)
	if err != nil {
		return err
	}
	for _, spanStore := range stores {
		pinger, ok := spanStore.(interface{ Ping(context.Context) error })
		if !ok {
			continue
		}
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Stats(ctx context.Context) (map[string]any, error) {
	stores, err := m.allSpanStores(ctx)
	if err != nil {
		return nil, err
	}
	stats := map[string]any{
		"backend": "manager",
		"stores":  len(stores),
	}
	children := make([]map[string]any, 0, len(stores))
	for _, spanStore := range stores {
		statter, ok := spanStore.(interface {
			Stats(context.Context) (map[string]any, error)
		})
		if !ok {
			continue
		}
		child, err := statter.Stats(ctx)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	if len(children) > 0 {
		stats["children"] = children
	}
	return stats, nil
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
		return clickhouseimpl.New(spec.DSN)

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

func newAuditStoreFromSpec(ctx context.Context, spec config.StoreSpec, postgresCfg config.PostgresConfig) (store.AuditStore, error) {
	switch spec.Kind {
	case "postgres":
		dsn := storeDSN(spec, postgresCfg)
		pool, err := postgresdb.NewWithDSN(ctx, dsn, postgresCfg)
		if err != nil {
			return nil, err
		}
		return postgresimpl.NewAuditStore(pool), nil

	case "sqlite", "":
		path := spec.Path
		if path == "" {
			path = "kave.db"
		}
		return sqliteimpl.NewAuditStoreFromPath(path)

	default:
		return nil, fmt.Errorf("unknown audit store backend %q", spec.Kind)
	}
}
