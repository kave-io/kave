package clickhouse

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
	"github.com/kave-io/kave/server/internal/store/spansql"
)

var _ store.SpanStore = (*SpanStore)(nil)

// SpanStore stores logical span rows in ClickHouse using immutable versions.
// OpenSpan inserts version=1 and CloseSpan inserts a merged version=2 row.
// ReplacingMergeTree(version) plus FINAL reads expose one logical row per span.
type SpanStore struct {
	db  *sql.DB
	dsn string
}

func New(dsn string) (*SpanStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("clickhouse: dsn is required")
	}
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("clickhouse: ping: %w", err)
	}
	return &SpanStore{db: db, dsn: dsn}, nil
}

func (s *SpanStore) Close() error { return s.db.Close() }

func (s *SpanStore) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS spans (
			id String,
			version UInt64,
			run_id String,
			action_id String,
			project_id String,
			env_id String,
			agent_id String,
			parent_id Nullable(String),
			name String,
			kind String,
			source String,
			connector String,
			started_at DateTime64(3, 'UTC'),
			ended_at Nullable(DateTime64(3, 'UTC')),
			duration_ms Int64,
			input Nullable(String),
			output Nullable(String),
			error Nullable(String),
			input_tokens Nullable(Int64),
			output_tokens Nullable(Int64),
			cache_read_tokens Nullable(Int64),
			cache_write_tokens Nullable(Int64),
			reasoning_tokens Nullable(Int64),
			audio_input_tokens Nullable(Int64),
			audio_output_tokens Nullable(Int64),
			image_units Nullable(Int64),
			request_count Nullable(Int64),
			compute_ms Nullable(Int64),
			storage_bytes Nullable(Int64),
			bandwidth_bytes Nullable(Int64),
			model Nullable(String),
			cost_amount_nanos Nullable(Int64),
			price_version Nullable(String),
			price_snapshot Nullable(String),
			attrs Nullable(String),
			trace_id String,
			root_span_id String,
			validation_meta Nullable(String),
			created_at DateTime64(3, 'UTC')
		)
		ENGINE = ReplacingMergeTree(version)
		PARTITION BY toYYYYMM(started_at)
		ORDER BY id
	`)
	if err != nil {
		return fmt.Errorf("clickhouse: migrate spans: %w", err)
	}
	return nil
}

func (s *SpanStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *SpanStore) Stats(ctx context.Context) (map[string]any, error) {
	var count int64
	if err := s.db.QueryRowContext(ctx, `SELECT count() FROM spans FINAL`).Scan(&count); err != nil {
		return nil, err
	}
	return map[string]any{
		"backend": "clickhouse",
		"dsn":     redactDSN(s.dsn),
		"tables":  map[string]int64{"spans": count},
	}, nil
}

func (s *SpanStore) OpenSpan(ctx context.Context, span *runtimemodel.SpanRow) error {
	return s.insertSpan(ctx, span, 1)
}

func (s *SpanStore) CloseSpan(ctx context.Context, spanID string, end *runtimemodel.SpanEnd) error {
	row, err := s.GetSpan(ctx, spanID)
	if err != nil {
		return err
	}
	if row == nil {
		return nil
	}
	mergeEnd(row, end)
	return s.insertSpan(ctx, row, 2)
}

func (s *SpanStore) GetSpan(ctx context.Context, spanID string) (*runtimemodel.SpanRow, error) {
	row := s.db.QueryRowContext(ctx, selectSpanSQL(`WHERE id = ? LIMIT 1`), spanID)
	span, err := scanSpan(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return span, nil
}

func (s *SpanStore) QuerySpans(ctx context.Context, filter *runtimemodel.SpanFilter, page store.Page) (store.PageResult[*runtimemodel.SpanRow], error) {
	whereSQL, args := spansql.BuildWhere(filter, spansql.ClickHouse)
	limit := page.Limit
	if limit <= 0 {
		limit = 100
	}
	query := selectSpanSQL("WHERE 1=1" + whereSQL + " ORDER BY started_at DESC LIMIT ?")
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.PageResult[*runtimemodel.SpanRow]{}, err
	}
	defer rows.Close()

	items := make([]*runtimemodel.SpanRow, 0)
	for rows.Next() {
		span, err := scanSpan(rows)
		if err != nil {
			return store.PageResult[*runtimemodel.SpanRow]{}, err
		}
		items = append(items, span)
	}
	return store.PageResult[*runtimemodel.SpanRow]{Items: items}, rows.Err()
}

func (s *SpanStore) SpendByDimension(ctx context.Context, groupBy string, filter *runtimemodel.SpanFilter) (map[string]money.Amount, error) {
	col, ok := allowedGroupBy[groupBy]
	if !ok {
		return nil, fmt.Errorf("invalid groupBy %q", groupBy)
	}
	whereSQL, args := spansql.BuildWhere(filter, spansql.ClickHouse)
	query := fmt.Sprintf(`
		SELECT ifNull(toString(%s), 'unknown'), ifNull(sum(cost_amount_nanos), 0)
		FROM spans FINAL WHERE 1=1%s
		GROUP BY %s ORDER BY sum(cost_amount_nanos) DESC
	`, col, whereSQL, col)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]money.Amount{}
	for rows.Next() {
		var dim string
		var cost int64
		if err := rows.Scan(&dim, &cost); err != nil {
			return nil, err
		}
		result[dim] = money.Amount(cost)
	}
	return result, rows.Err()
}

func (s *SpanStore) insertSpan(ctx context.Context, span *runtimemodel.SpanRow, version uint64) error {
	if span == nil {
		return fmt.Errorf("clickhouse: span is nil")
	}
	priceSnapshot := nullableJSON(span.PriceSnapshot)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO spans (
			id, version, run_id, action_id, project_id, env_id, agent_id, parent_id, name, kind, source, connector,
			started_at, ended_at, duration_ms, input, output, error,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens,
			audio_input_tokens, audio_output_tokens, image_units, request_count, compute_ms,
			storage_bytes, bandwidth_bytes, model, cost_amount_nanos, price_version, price_snapshot,
			attrs, trace_id, root_span_id, validation_meta, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		span.ID, version, span.RunID, span.ActionID, span.ProjectID, span.EnvID, span.AgentID, span.ParentID, span.Name, span.Kind, span.Source, span.Connector,
		time.UnixMilli(span.StartedAt), ptrTime(span.EndedAt), span.DurationMs, nullableBytes(span.Input), nullableBytes(span.Output), span.Error,
		ptrInt64(span.InputTokens), ptrInt64(span.OutputTokens), ptrInt64(span.CacheReadTokens), ptrInt64(span.CacheWriteTokens), ptrInt64(span.ReasoningTokens),
		ptrInt64(span.AudioInputTokens), ptrInt64(span.AudioOutputTokens), ptrInt64(span.ImageUnits), ptrInt64(span.RequestCount), span.ComputeMs,
		span.StorageBytes, span.BandwidthBytes, span.Model, ptrAmountToDB(span.Cost), span.PriceVersion, priceSnapshot,
		nullableBytes(span.Attrs), span.TraceID, span.RootSpanID, nullableRaw(span.ValidationMeta), time.UnixMilli(resolveCreatedAt(span.CreatedAt)),
	)
	if err != nil {
		return fmt.Errorf("clickhouse: insert span: %w", err)
	}
	return nil
}

func selectSpanSQL(suffix string) string {
	return `
		SELECT id, run_id, action_id, project_id, env_id, agent_id, parent_id, name, kind, source, connector,
		       started_at, ended_at, duration_ms, input, output, error,
		       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, reasoning_tokens,
		       audio_input_tokens, audio_output_tokens, image_units, request_count, compute_ms,
		       storage_bytes, bandwidth_bytes, model, cost_amount_nanos, price_version, price_snapshot,
		       attrs, trace_id, root_span_id, validation_meta, created_at
		FROM spans FINAL ` + suffix
}

type rowScanner interface{ Scan(dest ...any) error }

func scanSpan(scanner rowScanner) (*runtimemodel.SpanRow, error) {
	row := &runtimemodel.SpanRow{}
	var (
		parentID, input, output, errText, model, priceVersion, priceSnapshot, attrs, validationMeta sql.NullString
		startedAt, createdAt                                                                        time.Time
		endedAt                                                                                     sql.NullTime
		inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens                                sql.NullInt64
		reasoningTokens, audioInputTokens, audioOutputTokens, imageUnits, requestCount              sql.NullInt64
		computeMs, storageBytes, bandwidthBytes, costAmount                                         sql.NullInt64
	)
	if err := scanner.Scan(
		&row.ID, &row.RunID, &row.ActionID, &row.ProjectID, &row.EnvID, &row.AgentID, &parentID, &row.Name, &row.Kind, &row.Source, &row.Connector,
		&startedAt, &endedAt, &row.DurationMs, &input, &output, &errText,
		&inputTokens, &outputTokens, &cacheReadTokens, &cacheWriteTokens, &reasoningTokens,
		&audioInputTokens, &audioOutputTokens, &imageUnits, &requestCount, &computeMs,
		&storageBytes, &bandwidthBytes, &model, &costAmount, &priceVersion, &priceSnapshot,
		&attrs, &row.TraceID, &row.RootSpanID, &validationMeta, &createdAt,
	); err != nil {
		return nil, err
	}
	row.StartedAt = startedAt.UnixMilli()
	row.CreatedAt = createdAt.UnixMilli()
	if parentID.Valid {
		row.ParentID = &parentID.String
	}
	if endedAt.Valid {
		ms := endedAt.Time.UnixMilli()
		row.EndedAt = &ms
	}
	if input.Valid {
		b := []byte(input.String)
		row.Input = &b
	}
	if output.Valid {
		b := []byte(output.String)
		row.Output = &b
	}
	if attrs.Valid {
		b := []byte(attrs.String)
		row.Attrs = &b
	}
	if errText.Valid {
		row.Error = &errText.String
	}
	if model.Valid {
		row.Model = &model.String
	}
	if priceVersion.Valid {
		row.PriceVersion = &priceVersion.String
	}
	if validationMeta.Valid {
		row.ValidationMeta = []byte(validationMeta.String)
	}
	if priceSnapshot.Valid && priceSnapshot.String != "" {
		var ps runtimemodel.PriceSnapshot
		_ = json.Unmarshal([]byte(priceSnapshot.String), &ps)
		row.PriceSnapshot = &ps
	}
	setInt(&row.InputTokens, inputTokens)
	setInt(&row.OutputTokens, outputTokens)
	setInt(&row.CacheReadTokens, cacheReadTokens)
	setInt(&row.CacheWriteTokens, cacheWriteTokens)
	setInt(&row.ReasoningTokens, reasoningTokens)
	setInt(&row.AudioInputTokens, audioInputTokens)
	setInt(&row.AudioOutputTokens, audioOutputTokens)
	setInt(&row.ImageUnits, imageUnits)
	setInt(&row.RequestCount, requestCount)
	setInt64(&row.ComputeMs, computeMs)
	setInt64(&row.StorageBytes, storageBytes)
	setInt64(&row.BandwidthBytes, bandwidthBytes)
	if costAmount.Valid {
		amt := money.Amount(costAmount.Int64)
		row.Cost = &amt
	}
	return row, nil
}

func mergeEnd(row *runtimemodel.SpanRow, end *runtimemodel.SpanEnd) {
	if end == nil {
		return
	}
	if end.EndedAt != nil {
		row.EndedAt = end.EndedAt
	}
	row.DurationMs = end.DurationMs
	if end.Output != nil {
		row.Output = end.Output
	}
	if end.Attrs != nil {
		row.Attrs = end.Attrs
	}
	if end.Error != nil {
		row.Error = end.Error
	}
	if end.InputTokens != nil {
		row.InputTokens = end.InputTokens
	}
	if end.OutputTokens != nil {
		row.OutputTokens = end.OutputTokens
	}
	if end.CacheReadTokens != nil {
		row.CacheReadTokens = end.CacheReadTokens
	}
	if end.CacheWriteTokens != nil {
		row.CacheWriteTokens = end.CacheWriteTokens
	}
	if end.ReasoningTokens != nil {
		row.ReasoningTokens = end.ReasoningTokens
	}
	if end.AudioInputTokens != nil {
		row.AudioInputTokens = end.AudioInputTokens
	}
	if end.AudioOutputTokens != nil {
		row.AudioOutputTokens = end.AudioOutputTokens
	}
	if end.ImageUnits != nil {
		row.ImageUnits = end.ImageUnits
	}
	if end.RequestCount != nil {
		row.RequestCount = end.RequestCount
	}
	if end.ComputeMs != nil {
		row.ComputeMs = end.ComputeMs
	}
	if end.StorageBytes != nil {
		row.StorageBytes = end.StorageBytes
	}
	if end.BandwidthBytes != nil {
		row.BandwidthBytes = end.BandwidthBytes
	}
	if end.Model != nil {
		row.Model = end.Model
	}
	if end.Cost != nil {
		row.Cost = end.Cost
	}
	if end.PriceVersion != nil {
		row.PriceVersion = end.PriceVersion
	}
	if end.PriceSnapshot != nil {
		row.PriceSnapshot = end.PriceSnapshot
	}
	if end.TraceID != "" {
		row.TraceID = end.TraceID
	}
	if end.RootSpanID != "" {
		row.RootSpanID = end.RootSpanID
	}
	if len(end.ValidationMeta) > 0 {
		row.ValidationMeta = end.ValidationMeta
	}
}

func ptrTime(ms *int64) *time.Time {
	if ms == nil {
		return nil
	}
	t := time.UnixMilli(*ms)
	return &t
}

func nullableBytes(v *[]byte) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

func nullableRaw(v []byte) *string {
	if len(v) == 0 {
		return nil
	}
	s := string(v)
	return &s
}

func nullableJSON(v any) *string {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

func ptrInt64(v *int) *int64 {
	if v == nil {
		return nil
	}
	n := int64(*v)
	return &n
}

func ptrAmountToDB(v *money.Amount) *int64 {
	if v == nil {
		return nil
	}
	n := v.Nano()
	return &n
}

func setInt(dst **int, src sql.NullInt64) {
	if src.Valid {
		v := int(src.Int64)
		*dst = &v
	}
}

func setInt64(dst **int64, src sql.NullInt64) {
	if src.Valid {
		v := src.Int64
		*dst = &v
	}
}

func resolveCreatedAt(ms int64) int64 {
	if ms != 0 {
		return ms
	}
	return time.Now().UnixMilli()
}

func redactDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	if i := strings.Index(dsn, "password="); i >= 0 {
		end := strings.IndexAny(dsn[i:], "& ")
		if end < 0 {
			return dsn[:i] + "password=***"
		}
		return dsn[:i] + "password=***" + dsn[i+end:]
	}
	return dsn
}

var allowedGroupBy = map[string]string{
	"model":     "model",
	"run_id":    "run_id",
	"connector": "connector",
	"hour":      "toStartOfHour(started_at)",
	"day":       "toDate(started_at)",
}
