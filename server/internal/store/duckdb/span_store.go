// Package duckdb provides a DuckDB-backed SpanStore implementation.
// CGO_ENABLED=1 required — uses marcboeker/go-duckdb
package duckdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/store/spansql"
	_ "github.com/marcboeker/go-duckdb"
)

var _ store.SpanStore = (*DuckDBSpanStore)(nil)

const (
	defaultBufferSize    = 100
	defaultFlushInterval = 2 * time.Second
)

// DuckDBSpanStore implements store.SpanStore using DuckDB with a buffered channel writer.
// A single background goroutine owns the DuckDB connection and writes batches.
type DuckDBSpanStore struct {
	db   *sql.DB
	path string
	ch   chan writeReq
	done chan struct{}
	once sync.Once
	mu   sync.Mutex // guards reads
}

type writeReq struct {
	spanID string                // set for CloseSpan
	open   *runtimemodel.SpanRow // set for OpenSpan
	end    *runtimemodel.SpanEnd // set for CloseSpan
	respCh chan error
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
		db:   db,
		path: path,
		ch:   make(chan writeReq, defaultBufferSize),
		done: make(chan struct{}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("duckdb: migrate: %w", err)
	}

	go s.writerLoop()
	return s, nil
}

// Close closes the store and flushes remaining spans.
func (s *DuckDBSpanStore) Close() error {
	var err error
	s.once.Do(func() {
		close(s.done)
		time.Sleep(100 * time.Millisecond)
		err = s.db.Close()
	})
	return err
}

// Migrate runs pending migrations.
func (s *DuckDBSpanStore) Migrate(ctx context.Context) error {
	return Migrate(ctx, s.db)
}

func (s *DuckDBSpanStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *DuckDBSpanStore) Stats(ctx context.Context) (map[string]any, error) {
	stats := map[string]any{
		"backend": "duckdb",
		"path":    s.path,
	}
	if info, err := os.Stat(s.path); err == nil {
		stats["size_bytes"] = info.Size()
	}
	var count int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM spans`).Scan(&count); err == nil {
		stats["tables"] = map[string]int64{"spans": count}
	}
	return stats, nil
}

// OpenSpan inserts a new span (buffered, async).
func (s *DuckDBSpanStore) OpenSpan(ctx context.Context, span *runtimemodel.SpanRow) error {
	respCh := make(chan error, 1)
	select {
	case s.ch <- writeReq{open: span, respCh: respCh}:
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

// CloseSpan updates an existing span with its final fields (buffered, async).
func (s *DuckDBSpanStore) CloseSpan(ctx context.Context, spanID string, end *runtimemodel.SpanEnd) error {
	respCh := make(chan error, 1)
	select {
	case s.ch <- writeReq{spanID: spanID, end: end, respCh: respCh}:
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
func (s *DuckDBSpanStore) GetSpan(ctx context.Context, spanID string) (*runtimemodel.SpanRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := &runtimemodel.SpanRow{}
	var (
		input, output, attrs []byte
		endedAt              sql.NullInt64
		model                sql.NullString
		costAmount           sql.NullInt64
		priceVersion         sql.NullString
		snapshotJSON         sql.NullString
		validationMeta       sql.NullString
		inputTokens          sql.NullInt32
		outputTokens         sql.NullInt32
		cacheReadTokens      sql.NullInt32
		cacheWriteTokens     sql.NullInt32
		reasoningTokens      sql.NullInt32
		audioInputTokens     sql.NullInt32
		audioOutputTokens    sql.NullInt32
		imageUnits           sql.NullInt32
		requestCount         sql.NullInt32
		computeMs            sql.NullInt64
		storageBytes         sql.NullInt64
		bandwidthBytes       sql.NullInt64
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, run_id, action_id, project_id, env_id, agent_id, parent_id, name, kind, source, connector, started_at, ended_at, duration_ms,
		       input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		       model, cost_amount_nanos, price_version, price_snapshot, attrs, created_at,
		       reasoning_tokens, audio_input_tokens, audio_output_tokens, image_units, request_count,
		       compute_ms, storage_bytes, bandwidth_bytes, trace_id, root_span_id, validation_meta
		FROM spans WHERE id = $1
	`, spanID).Scan(
		&row.ID, &row.RunID, &row.ActionID, &row.ProjectID, &row.EnvID, &row.AgentID, &row.ParentID, &row.Name, &row.Kind, &row.Source, &row.Connector, &row.StartedAt, &endedAt, &row.DurationMs,
		&input, &output, &row.Error, &inputTokens, &outputTokens, &cacheReadTokens, &cacheWriteTokens,
		&model, &costAmount, &priceVersion, &snapshotJSON, &attrs, &row.CreatedAt,
		&reasoningTokens, &audioInputTokens, &audioOutputTokens, &imageUnits, &requestCount,
		&computeMs, &storageBytes, &bandwidthBytes, &row.TraceID, &row.RootSpanID, &validationMeta,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if endedAt.Valid {
		row.EndedAt = &endedAt.Int64
	}
	if input != nil {
		row.Input = &input
	}
	if output != nil {
		row.Output = &output
	}
	if attrs != nil {
		row.Attrs = &attrs
	}
	if model.Valid {
		row.Model = &model.String
	}
	if costAmount.Valid {
		amt := amountFromDB(costAmount.Int64)
		row.Cost = &amt
	}
	if priceVersion.Valid {
		row.PriceVersion = &priceVersion.String
	}
	if snapshotJSON.Valid && snapshotJSON.String != "" {
		var ps runtimemodel.PriceSnapshot
		_ = json.Unmarshal([]byte(snapshotJSON.String), &ps)
		row.PriceSnapshot = &ps
	}
	if validationMeta.Valid {
		row.ValidationMeta = []byte(validationMeta.String)
	}
	setNullInt32(&row.InputTokens, inputTokens)
	setNullInt32(&row.OutputTokens, outputTokens)
	setNullInt32(&row.CacheReadTokens, cacheReadTokens)
	setNullInt32(&row.CacheWriteTokens, cacheWriteTokens)
	setNullInt32(&row.ReasoningTokens, reasoningTokens)
	setNullInt32(&row.AudioInputTokens, audioInputTokens)
	setNullInt32(&row.AudioOutputTokens, audioOutputTokens)
	setNullInt32(&row.ImageUnits, imageUnits)
	setNullInt32(&row.RequestCount, requestCount)
	if computeMs.Valid {
		row.ComputeMs = &computeMs.Int64
	}
	if storageBytes.Valid {
		row.StorageBytes = &storageBytes.Int64
	}
	if bandwidthBytes.Valid {
		row.BandwidthBytes = &bandwidthBytes.Int64
	}

	return row, nil
}

// QuerySpans retrieves spans matching a filter.
func (s *DuckDBSpanStore) QuerySpans(ctx context.Context, filter *runtimemodel.SpanFilter, page store.Page) (store.PageResult[*runtimemodel.SpanRow], error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		SELECT id, run_id, action_id, project_id, env_id, agent_id, parent_id, name, kind, source, connector, started_at, ended_at, duration_ms,
		       input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		       model, cost_amount_nanos, attrs, created_at
		FROM spans WHERE 1=1
	`
	whereSQL, whereArgs := spansql.BuildWhere(filter, spansql.DuckDB)
	query += whereSQL
	args := append([]any{}, whereArgs...)

	limit := page.Limit
	if limit <= 0 {
		limit = 100
	}
	query += ` ORDER BY started_at DESC LIMIT $` + fmt.Sprint(len(args)+1)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.PageResult[*runtimemodel.SpanRow]{}, err
	}
	defer rows.Close()

	var spans []*runtimemodel.SpanRow
	for rows.Next() {
		row := &runtimemodel.SpanRow{}
		var (
			input, output, attrs []byte
			endedAt              sql.NullInt64
			model                sql.NullString
			costAmount           sql.NullInt64
			inputTokens          sql.NullInt32
			outputTokens         sql.NullInt32
			cacheReadTokens      sql.NullInt32
			cacheWriteTokens     sql.NullInt32
		)
		if err := rows.Scan(
			&row.ID, &row.RunID, &row.ActionID, &row.ProjectID, &row.EnvID, &row.AgentID, &row.ParentID, &row.Name, &row.Kind, &row.Source, &row.Connector, &row.StartedAt, &endedAt, &row.DurationMs,
			&input, &output, &row.Error, &inputTokens, &outputTokens, &cacheReadTokens, &cacheWriteTokens,
			&model, &costAmount, &attrs, &row.CreatedAt,
		); err != nil {
			return store.PageResult[*runtimemodel.SpanRow]{}, err
		}
		if endedAt.Valid {
			row.EndedAt = &endedAt.Int64
		}
		if input != nil {
			row.Input = &input
		}
		if output != nil {
			row.Output = &output
		}
		if attrs != nil {
			row.Attrs = &attrs
		}
		if model.Valid {
			row.Model = &model.String
		}
		if costAmount.Valid {
			amt := amountFromDB(costAmount.Int64)
			row.Cost = &amt
		}
		setNullInt32(&row.InputTokens, inputTokens)
		setNullInt32(&row.OutputTokens, outputTokens)
		setNullInt32(&row.CacheReadTokens, cacheReadTokens)
		setNullInt32(&row.CacheWriteTokens, cacheWriteTokens)
		spans = append(spans, row)
	}
	return store.PageResult[*runtimemodel.SpanRow]{Items: spans}, rows.Err()
}

// SpendByDimension aggregates cost by the given dimension.
func (s *DuckDBSpanStore) SpendByDimension(ctx context.Context, groupBy string, filter *runtimemodel.SpanFilter) (map[string]money.Amount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	col, ok := allowedGroupBy[groupBy]
	if !ok {
		return nil, fmt.Errorf("invalid groupBy %q", groupBy)
	}

	query := fmt.Sprintf(`
		SELECT %s, COALESCE(SUM(cost_amount_nanos), 0)
		FROM spans WHERE 1=1
	`, col)
	whereSQL, whereArgs := spansql.BuildWhere(filter, spansql.DuckDB)
	query += whereSQL
	args := append([]any{}, whereArgs...)
	query += fmt.Sprintf(` GROUP BY %s ORDER BY SUM(cost_amount_nanos) DESC`, col)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]money.Amount)
	for rows.Next() {
		var dimension string
		var cost int64
		if err := rows.Scan(&dimension, &cost); err != nil {
			return nil, err
		}
		result[dimension] = amountFromDB(cost)
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
			if req.open != nil {
				execErr = insertSpan(tx, req.open)
			} else if req.end != nil {
				execErr = updateSpan(tx, req.spanID, req.end)
			}
			req.respCh <- execErr
		}

		_ = tx.Commit()
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
			flush()
			return
		}
	}
}

func insertSpan(tx *sql.Tx, span *runtimemodel.SpanRow) error {
	costAmount := ptrAmountToDB(span.Cost)
	var snapshotJSON *string
	if span.PriceSnapshot != nil {
		b, _ := json.Marshal(span.PriceSnapshot)
		s := string(b)
		snapshotJSON = &s
	}
	var validationMeta *string
	if len(span.ValidationMeta) > 0 {
		s := string(span.ValidationMeta)
		validationMeta = &s
	}

	_, err := tx.Exec(`
		INSERT INTO spans (id, run_id, action_id, project_id, env_id, agent_id, parent_id, name, kind, source, connector, started_at, ended_at, duration_ms,
		                   input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		                   model, cost_amount_nanos, price_version, price_snapshot, attrs, created_at,
		                   reasoning_tokens, audio_input_tokens, audio_output_tokens, image_units, request_count,
		                   compute_ms, storage_bytes, bandwidth_bytes, trace_id, root_span_id, validation_meta)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		span.ID, span.RunID, span.ActionID, span.ProjectID, span.EnvID, span.AgentID, span.ParentID, span.Name, span.Kind, span.Source, span.Connector, span.StartedAt, span.EndedAt, span.DurationMs,
		derefBytes(span.Input), derefBytes(span.Output), span.Error,
		span.InputTokens, span.OutputTokens, span.CacheReadTokens, span.CacheWriteTokens,
		span.Model, costAmount, span.PriceVersion, snapshotJSON, derefBytes(span.Attrs), time.Now().UnixMilli(),
		span.ReasoningTokens, span.AudioInputTokens, span.AudioOutputTokens, span.ImageUnits, span.RequestCount,
		span.ComputeMs, span.StorageBytes, span.BandwidthBytes, span.TraceID, span.RootSpanID, validationMeta,
	)
	return err
}

func updateSpan(tx *sql.Tx, spanID string, end *runtimemodel.SpanEnd) error {
	costAmount := ptrAmountToDB(end.Cost)
	var snapshotJSON *string
	if end.PriceSnapshot != nil {
		b, _ := json.Marshal(end.PriceSnapshot)
		s := string(b)
		snapshotJSON = &s
	}
	var validationMeta *string
	if len(end.ValidationMeta) > 0 {
		s := string(end.ValidationMeta)
		validationMeta = &s
	}
	var traceID, rootSpanID *string
	if end.TraceID != "" {
		traceID = &end.TraceID
	}
	if end.RootSpanID != "" {
		rootSpanID = &end.RootSpanID
	}

	_, err := tx.Exec(`
		UPDATE spans SET
			ended_at          = ?,
			duration_ms       = ?,
			output            = ?,
			attrs             = ?,
			error             = ?,
			input_tokens      = COALESCE(?, input_tokens),
			output_tokens     = COALESCE(?, output_tokens),
			cache_read_tokens = COALESCE(?, cache_read_tokens),
			cache_write_tokens= COALESCE(?, cache_write_tokens),
			reasoning_tokens  = COALESCE(?, reasoning_tokens),
			audio_input_tokens= COALESCE(?, audio_input_tokens),
			audio_output_tokens= COALESCE(?, audio_output_tokens),
			image_units       = COALESCE(?, image_units),
			request_count     = COALESCE(?, request_count),
			compute_ms        = COALESCE(?, compute_ms),
			storage_bytes     = COALESCE(?, storage_bytes),
			bandwidth_bytes   = COALESCE(?, bandwidth_bytes),
			model             = COALESCE(?, model),
			cost_amount_nanos = COALESCE(?, cost_amount_nanos),
			price_version     = COALESCE(?, price_version),
			price_snapshot    = COALESCE(?, price_snapshot),
			trace_id          = COALESCE(?, trace_id),
			root_span_id      = COALESCE(?, root_span_id),
		validation_meta   = COALESCE(?, validation_meta)
		WHERE id = ?
	`,
		end.EndedAt, end.DurationMs,
		derefBytes(end.Output), derefBytes(end.Attrs), end.Error,
		end.InputTokens, end.OutputTokens, end.CacheReadTokens, end.CacheWriteTokens,
		end.ReasoningTokens, end.AudioInputTokens, end.AudioOutputTokens, end.ImageUnits, end.RequestCount,
		end.ComputeMs, end.StorageBytes, end.BandwidthBytes,
		end.Model, costAmount, end.PriceVersion, snapshotJSON,
		traceID, rootSpanID, validationMeta,
		spanID,
	)
	return err
}

// Allowlist for groupBy values in SpendByDimension.
var allowedGroupBy = map[string]string{
	"model":     "COALESCE(model, '')",
	"run_id":    "run_id",
	"connector": "connector",
	"hour":      "date_trunc('hour', to_timestamp(started_at / 1000.0))",
	"day":       "date_trunc('day', to_timestamp(started_at / 1000.0))",
}

func derefBytes(b *[]byte) []byte {
	if b == nil {
		return nil
	}
	return *b
}

func setNullInt32(dst **int, v sql.NullInt32) {
	if v.Valid {
		i := int(v.Int32)
		*dst = &i
	}
}

func amountToDB(v money.Amount) int64 { return v.Nano() }

func ptrAmountToDB(v *money.Amount) *int64 {
	if v == nil {
		return nil
	}
	n := v.Nano()
	return &n
}

func amountFromDB(v int64) money.Amount { return money.Amount(v) }
