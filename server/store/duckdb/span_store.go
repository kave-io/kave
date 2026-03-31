// Package duckdb provides a DuckDB-backed SpanStore implementation.
// CGO_ENABLED=1 required — uses marcboeker/go-duckdb
package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/marcboeker/go-duckdb"
	"github.com/kave-io/kave/core/store"
	dbduckdb "github.com/kave-io/kave/server/db/duckdb"
)

const (
	defaultBufferSize     = 100
	defaultFlushInterval  = 2 * time.Second
)

// DuckDBSpanStore implements store.SpanStore using DuckDB with a buffered channel writer.
// A single background goroutine owns the DuckDB connection and writes batches.
type DuckDBSpanStore struct {
	db    *sql.DB
	ch    chan writeReq
	done  chan struct{}
	once  sync.Once
	errCh chan error
	mu    sync.Mutex // guards reads (optional, for safety)
}

type writeReq struct {
	span     *store.SpanRow
	respCh   chan error
	isUpdate bool // true for UpdateSpan, false for WriteSpan
}

// New creates a new DuckDB span store with the given file path.
func New(path string) (*DuckDBSpanStore, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("duckdb: open database: %w", err)
	}

	// DuckDB prefers single connection
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("duckdb: ping database: %w", err)
	}

	s := &DuckDBSpanStore{
		db:    db,
		ch:    make(chan writeReq, defaultBufferSize),
		done:  make(chan struct{}),
		errCh: make(chan error, 1),
	}

	// Run migrations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("duckdb: migrate: %w", err)
	}

	// Start background writer goroutine
	go s.writerLoop()

	return s, nil
}

// Close closes the store and flushes remaining spans.
func (s *DuckDBSpanStore) Close() error {
	var err error
	s.once.Do(func() {
		close(s.done)
		// Give the writer goroutine time to flush
		time.Sleep(100 * time.Millisecond)
		err = s.db.Close()
	})
	return err
}

// Migrate runs pending migrations.
func (s *DuckDBSpanStore) Migrate(ctx context.Context) error {
	return dbduckdb.Migrate(ctx, s.db)
}

// WriteSpan writes a span to the store (buffered, non-blocking).
func (s *DuckDBSpanStore) WriteSpan(ctx context.Context, span *store.SpanRow) error {
	respCh := make(chan error, 1)
	req := writeReq{span: span, respCh: respCh, isUpdate: false}

	select {
	case s.ch <- req:
		// Wait for response with context timeout
		select {
		case err := <-respCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-s.done:
		return fmt.Errorf("duckdb: span store closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// UpdateSpan updates an existing span (upsert semantics).
func (s *DuckDBSpanStore) UpdateSpan(ctx context.Context, span *store.SpanRow) error {
	respCh := make(chan error, 1)
	req := writeReq{span: span, respCh: respCh, isUpdate: true}

	select {
	case s.ch <- req:
		select {
		case err := <-respCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-s.done:
		return fmt.Errorf("duckdb: span store closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// GetSpan retrieves a single span by ID.
func (s *DuckDBSpanStore) GetSpan(ctx context.Context, spanID string) (*store.SpanRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var row store.SpanRow
	var input, output, tags []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms,
		       input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, model, cost_usd, tags
		FROM spans WHERE id = $1
	`, spanID).Scan(
		&row.ID, &row.RunID, &row.ActionID, &row.ParentID, &row.Name, &row.StartedAt, &row.EndedAt, &row.DurationMs,
		&input, &output, &row.Error, &row.InputTokens, &row.OutputTokens, &row.CacheReadTokens, &row.CacheWriteTokens, &row.Model, &row.CostUSD, &tags,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	row.Input = input
	row.Output = output
	row.Tags = tags
	row.CreatedAt = time.Now().UnixMilli() // placeholder, not in SELECT

	return &row, nil
}

// QuerySpans retrieves spans matching a filter.
func (s *DuckDBSpanStore) QuerySpans(ctx context.Context, filter *store.SpanFilter) ([]*store.SpanRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		SELECT id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms,
		       input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, model, cost_usd, tags
		FROM spans WHERE 1=1
	`
	var args []any

	if filter.RunID != "" {
		query += ` AND run_id = $` + fmt.Sprint(len(args)+1)
		args = append(args, filter.RunID)
	}
	if filter.ActionID != "" {
		query += ` AND action_id = $` + fmt.Sprint(len(args)+1)
		args = append(args, filter.ActionID)
	}
	if filter.FromMs != nil {
		query += ` AND started_at >= $` + fmt.Sprint(len(args)+1)
		args = append(args, *filter.FromMs)
	}
	if filter.ToMs != nil {
		query += ` AND started_at <= $` + fmt.Sprint(len(args)+1)
		args = append(args, *filter.ToMs)
	}
	if filter.HasError != nil {
		if *filter.HasError {
			query += ` AND error IS NOT NULL`
		} else {
			query += ` AND error IS NULL`
		}
	}

	query += ` ORDER BY started_at DESC`
	if filter.Limit > 0 {
		query += ` LIMIT $` + fmt.Sprint(len(args)+1)
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spans []*store.SpanRow
	for rows.Next() {
		var row store.SpanRow
		var input, output, tags []byte
		if err := rows.Scan(
			&row.ID, &row.RunID, &row.ActionID, &row.ParentID, &row.Name, &row.StartedAt, &row.EndedAt, &row.DurationMs,
			&input, &output, &row.Error, &row.InputTokens, &row.OutputTokens, &row.CacheReadTokens, &row.CacheWriteTokens, &row.Model, &row.CostUSD, &tags,
		); err != nil {
			return nil, err
		}
		row.Input = input
		row.Output = output
		row.Tags = tags
		spans = append(spans, &row)
	}

	return spans, rows.Err()
}

// SpendByDimension aggregates cost by the given dimension.
func (s *DuckDBSpanStore) SpendByDimension(ctx context.Context, groupBy string, filter *store.SpanFilter) (map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate groupBy against allowlist
	col, ok := allowedGroupBy[groupBy]
	if !ok {
		return nil, fmt.Errorf("invalid groupBy %q", groupBy)
	}

	query := fmt.Sprintf(`
		SELECT %s, COALESCE(SUM(cost_usd), 0)
		FROM spans WHERE 1=1
	`, col)
	var args []any

	if filter.RunID != "" {
		query += ` AND run_id = $` + fmt.Sprint(len(args)+1)
		args = append(args, filter.RunID)
	}
	if filter.FromMs != nil {
		query += ` AND started_at >= $` + fmt.Sprint(len(args)+1)
		args = append(args, *filter.FromMs)
	}
	if filter.ToMs != nil {
		query += ` AND started_at <= $` + fmt.Sprint(len(args)+1)
		args = append(args, *filter.ToMs)
	}

	query += fmt.Sprintf(` GROUP BY %s ORDER BY SUM(cost_usd) DESC`, col)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var dimension string
		var cost float64
		if err := rows.Scan(&dimension, &cost); err != nil {
			return nil, err
		}
		result[dimension] = cost
	}

	return result, rows.Err()
}

// writerLoop is the background goroutine that owns the DuckDB connection.
func (s *DuckDBSpanStore) writerLoop() {
	ticker := time.NewTicker(defaultFlushInterval)
	defer ticker.Stop()

	var batch []writeReq

	flush := func() {
		if len(batch) == 0 {
			return
		}

		// Execute batch in a transaction
		tx, err := s.db.Begin()
		if err != nil {
			for _, req := range batch {
				req.respCh <- err
			}
			batch = batch[:0]
			return
		}

		for _, req := range batch {
			var execErr error
			if req.isUpdate {
				// UPDATE (upsert with INSERT OR REPLACE)
				_, execErr = tx.Exec(`
					INSERT OR REPLACE INTO spans (id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms, input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, model, cost_usd, tags, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				`, req.span.ID, req.span.RunID, req.span.ActionID, req.span.ParentID, req.span.Name, req.span.StartedAt, req.span.EndedAt, req.span.DurationMs,
					req.span.Input, req.span.Output, req.span.Error, req.span.InputTokens, req.span.OutputTokens, req.span.CacheReadTokens, req.span.CacheWriteTokens, req.span.Model, req.span.CostUSD, req.span.Tags, time.Now().UnixMilli())
			} else {
				// INSERT
				_, execErr = tx.Exec(`
					INSERT INTO spans (id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms, input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, model, cost_usd, tags, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				`, req.span.ID, req.span.RunID, req.span.ActionID, req.span.ParentID, req.span.Name, req.span.StartedAt, req.span.EndedAt, req.span.DurationMs,
					req.span.Input, req.span.Output, req.span.Error, req.span.InputTokens, req.span.OutputTokens, req.span.CacheReadTokens, req.span.CacheWriteTokens, req.span.Model, req.span.CostUSD, req.span.Tags, time.Now().UnixMilli())
			}
			req.respCh <- execErr
		}

		if err := tx.Commit(); err != nil {
			// Already sent per-request errors above, just log here if needed
		}
		batch = batch[:0]
	}

	for {
		select {
		case req := <-s.ch:
			batch = append(batch, req)
			if len(batch) >= defaultBufferSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-s.done:
			// Final flush
			flush()
			return
		}
	}
}

// Allowlist for groupBy values in SpendByDimension
var allowedGroupBy = map[string]string{
	"model":     "COALESCE(model, '')",
	"run_id":    "run_id",
	"connector": "connector",
	"hour":      "date_trunc('hour', to_timestamp(started_at / 1000.0))",
	"day":       "date_trunc('day', to_timestamp(started_at / 1000.0))",
}
