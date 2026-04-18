package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	runtimemodel "github.com/kave-io/kave/core/model/runtime"
	"github.com/kave-io/kave/core/pkg/money"
	"github.com/kave-io/kave/core/store"
)

var _ store.SpanStore = (*PostgresSpanStore)(nil)

// PostgresSpanStore implements store.SpanStore using Postgres.
// Unlike DuckDB, Postgres handles concurrent writes natively so no buffering is needed.
type PostgresSpanStore struct {
	pool *pgxpool.Pool
}

// NewSpanStore creates a new Postgres span store.
func NewSpanStore(pool *pgxpool.Pool) *PostgresSpanStore {
	return &PostgresSpanStore{pool: pool}
}

// Close closes the connection pool.
func (p *PostgresSpanStore) Close() error {
	p.pool.Close()
	return nil
}

// Migrate is a no-op for Postgres (migrations are handled by the main pool).
func (p *PostgresSpanStore) Migrate(ctx context.Context) error {
	return nil
}

// OpenSpan inserts a new span.
func (p *PostgresSpanStore) OpenSpan(ctx context.Context, span *runtimemodel.SpanRow) error {
	costAmount := ptrAmountToDB(span.Cost)
	var snapshotJSON []byte
	if span.PriceSnapshot != nil {
		snapshotJSON, _ = json.Marshal(span.PriceSnapshot)
	}
	var validationMeta []byte
	if len(span.ValidationMeta) > 0 {
		validationMeta = span.ValidationMeta
	}

	_, err := p.pool.Exec(ctx, `
		INSERT INTO spans (id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms,
		                   input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		                   model, cost_amount_nanos, price_version, price_snapshot, attrs, created_at,
		                   reasoning_tokens, audio_input_tokens, audio_output_tokens, image_units, request_count,
		                   compute_ms, storage_bytes, bandwidth_bytes, trace_id, root_span_id, validation_meta)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32)
	`,
		span.ID, span.RunID, span.ActionID, span.ParentID, span.Name,
		pgTime(span.StartedAt), ptrPgTime(span.EndedAt), span.DurationMs,
		derefBytesP(span.Input), derefBytesP(span.Output), span.Error,
		span.InputTokens, span.OutputTokens, span.CacheReadTokens, span.CacheWriteTokens,
		span.Model, costAmount, span.PriceVersion, snapshotJSON, derefBytesP(span.Attrs),
		time.Now(),
		span.ReasoningTokens, span.AudioInputTokens, span.AudioOutputTokens, span.ImageUnits, span.RequestCount,
		span.ComputeMs, span.StorageBytes, span.BandwidthBytes,
		span.TraceID, span.RootSpanID, validationMeta,
	)
	return err
}

// CloseSpan updates an existing span with its final fields.
func (p *PostgresSpanStore) CloseSpan(ctx context.Context, spanID string, end *runtimemodel.SpanEnd) error {
	costAmount := ptrAmountToDB(end.Cost)
	var snapshotJSON []byte
	if end.PriceSnapshot != nil {
		snapshotJSON, _ = json.Marshal(end.PriceSnapshot)
	}
	var validationMeta []byte
	if len(end.ValidationMeta) > 0 {
		validationMeta = end.ValidationMeta
	}
	var traceID, rootSpanID *string
	if end.TraceID != "" {
		traceID = &end.TraceID
	}
	if end.RootSpanID != "" {
		rootSpanID = &end.RootSpanID
	}

	_, err := p.pool.Exec(ctx, `
		UPDATE spans SET
			ended_at           = COALESCE($2, ended_at),
			duration_ms        = COALESCE($3, duration_ms),
			output             = COALESCE($4, output),
			attrs              = COALESCE($5, attrs),
			error              = COALESCE($6, error),
			input_tokens       = COALESCE($7, input_tokens),
			output_tokens      = COALESCE($8, output_tokens),
			cache_read_tokens  = COALESCE($9, cache_read_tokens),
			cache_write_tokens = COALESCE($10, cache_write_tokens),
			reasoning_tokens   = COALESCE($11, reasoning_tokens),
			audio_input_tokens = COALESCE($12, audio_input_tokens),
			audio_output_tokens= COALESCE($13, audio_output_tokens),
			image_units        = COALESCE($14, image_units),
			request_count      = COALESCE($15, request_count),
			compute_ms         = COALESCE($16, compute_ms),
			storage_bytes      = COALESCE($17, storage_bytes),
			bandwidth_bytes    = COALESCE($18, bandwidth_bytes),
			model              = COALESCE($19, model),
			cost_amount_nanos  = COALESCE($20, cost_amount_nanos),
			price_version      = COALESCE($21, price_version),
			price_snapshot     = COALESCE($22, price_snapshot),
			trace_id           = COALESCE($23, trace_id),
			root_span_id       = COALESCE($24, root_span_id),
			validation_meta    = COALESCE($25, validation_meta)
		WHERE id = $1
	`,
		spanID, ptrPgTime(end.EndedAt), end.DurationMs,
		derefBytesP(end.Output), derefBytesP(end.Attrs), end.Error,
		end.InputTokens, end.OutputTokens, end.CacheReadTokens, end.CacheWriteTokens,
		end.ReasoningTokens, end.AudioInputTokens, end.AudioOutputTokens, end.ImageUnits, end.RequestCount,
		end.ComputeMs, end.StorageBytes, end.BandwidthBytes,
		end.Model, costAmount, end.PriceVersion, snapshotJSON,
		traceID, rootSpanID, validationMeta,
	)
	return err
}

// GetSpan retrieves a single span by ID.
func (p *PostgresSpanStore) GetSpan(ctx context.Context, spanID string) (*runtimemodel.SpanRow, error) {
	row := &runtimemodel.SpanRow{}
	var (
		startedAt, createdAt time.Time
		endedAt              *time.Time
		model                *string
		costAmount           *int64
		priceVersion         *string
		snapshotJSON         []byte
		validationMeta       []byte
		input, output, attrs []byte
		inputTokens          *int32
		outputTokens         *int32
		cacheReadTokens      *int32
		cacheWriteTokens     *int32
		reasoningTokens      *int32
		audioInputTokens     *int32
		audioOutputTokens    *int32
		imageUnits           *int32
		requestCount         *int32
		computeMs            *int64
		storageBytes         *int64
		bandwidthBytes       *int64
	)

	err := p.pool.QueryRow(ctx, `
		SELECT id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms,
		       input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		       model, cost_amount_nanos, price_version, price_snapshot, attrs, created_at,
		       reasoning_tokens, audio_input_tokens, audio_output_tokens, image_units, request_count,
		       compute_ms, storage_bytes, bandwidth_bytes, trace_id, root_span_id, validation_meta
		FROM spans WHERE id = $1
	`, spanID).Scan(
		&row.ID, &row.RunID, &row.ActionID, &row.ParentID, &row.Name, &startedAt, &endedAt, &row.DurationMs,
		&input, &output, &row.Error, &inputTokens, &outputTokens, &cacheReadTokens, &cacheWriteTokens,
		&model, &costAmount, &priceVersion, &snapshotJSON, &attrs, &createdAt,
		&reasoningTokens, &audioInputTokens, &audioOutputTokens, &imageUnits, &requestCount,
		&computeMs, &storageBytes, &bandwidthBytes, &row.TraceID, &row.RootSpanID, &validationMeta,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	row.StartedAt = startedAt.UnixMilli()
	row.CreatedAt = createdAt.UnixMilli()
	if endedAt != nil {
		ms := endedAt.UnixMilli()
		row.EndedAt = &ms
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
	row.Model = model
	row.PriceVersion = priceVersion
	row.Cost = ptrAmountFromDB(costAmount)
	if len(snapshotJSON) > 0 {
		var ps runtimemodel.PriceSnapshot
		_ = json.Unmarshal(snapshotJSON, &ps)
		row.PriceSnapshot = &ps
	}
	if len(validationMeta) > 0 {
		row.ValidationMeta = validationMeta
	}
	setPtrInt32(&row.InputTokens, inputTokens)
	setPtrInt32(&row.OutputTokens, outputTokens)
	setPtrInt32(&row.CacheReadTokens, cacheReadTokens)
	setPtrInt32(&row.CacheWriteTokens, cacheWriteTokens)
	setPtrInt32(&row.ReasoningTokens, reasoningTokens)
	setPtrInt32(&row.AudioInputTokens, audioInputTokens)
	setPtrInt32(&row.AudioOutputTokens, audioOutputTokens)
	setPtrInt32(&row.ImageUnits, imageUnits)
	setPtrInt32(&row.RequestCount, requestCount)
	row.ComputeMs = computeMs
	row.StorageBytes = storageBytes
	row.BandwidthBytes = bandwidthBytes

	return row, nil
}

// QuerySpans retrieves spans matching a filter.
func (p *PostgresSpanStore) QuerySpans(ctx context.Context, filter *runtimemodel.SpanFilter, page store.Page) (store.PageResult[*runtimemodel.SpanRow], error) {
	query := `
		SELECT id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms,
		       input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		       model, cost_amount_nanos, attrs, created_at
		FROM spans WHERE 1=1
	`
	var args []any
	argNum := 1

	if filter.ID != "" {
		query += fmt.Sprintf(` AND id = $%d`, argNum)
		args = append(args, filter.ID)
		argNum++
	}
	if filter.RunID != "" {
		query += fmt.Sprintf(` AND run_id = $%d`, argNum)
		args = append(args, filter.RunID)
		argNum++
	}
	if filter.ActionID != "" {
		query += fmt.Sprintf(` AND action_id = $%d`, argNum)
		args = append(args, filter.ActionID)
		argNum++
	}
	if filter.FromMs != nil {
		query += fmt.Sprintf(` AND started_at >= $%d`, argNum)
		args = append(args, pgTime(*filter.FromMs))
		argNum++
	}
	if filter.ToMs != nil {
		query += fmt.Sprintf(` AND started_at <= $%d`, argNum)
		args = append(args, pgTime(*filter.ToMs))
		argNum++
	}
	if filter.HasError != nil {
		if *filter.HasError {
			query += ` AND error IS NOT NULL`
		} else {
			query += ` AND error IS NULL`
		}
	}

	limit := page.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(` ORDER BY started_at DESC LIMIT $%d`, argNum)
	args = append(args, limit)

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return store.PageResult[*runtimemodel.SpanRow]{}, err
	}
	defer rows.Close()

	var spans []*runtimemodel.SpanRow
	for rows.Next() {
		row := &runtimemodel.SpanRow{}
		var (
			startedAt, createdAt time.Time
			endedAt              *time.Time
			model                *string
			costAmount           *int64
			input, output, attrs []byte
			inputTokens          *int32
			outputTokens         *int32
			cacheReadTokens      *int32
			cacheWriteTokens     *int32
		)
		if err := rows.Scan(
			&row.ID, &row.RunID, &row.ActionID, &row.ParentID, &row.Name, &startedAt, &endedAt, &row.DurationMs,
			&input, &output, &row.Error, &inputTokens, &outputTokens, &cacheReadTokens, &cacheWriteTokens,
			&model, &costAmount, &attrs, &createdAt,
		); err != nil {
			return store.PageResult[*runtimemodel.SpanRow]{}, err
		}
		row.StartedAt = startedAt.UnixMilli()
		row.CreatedAt = createdAt.UnixMilli()
		if endedAt != nil {
			ms := endedAt.UnixMilli()
			row.EndedAt = &ms
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
		row.Model = model
		row.Cost = ptrAmountFromDB(costAmount)
		setPtrInt32(&row.InputTokens, inputTokens)
		setPtrInt32(&row.OutputTokens, outputTokens)
		setPtrInt32(&row.CacheReadTokens, cacheReadTokens)
		setPtrInt32(&row.CacheWriteTokens, cacheWriteTokens)
		spans = append(spans, row)
	}
	return store.PageResult[*runtimemodel.SpanRow]{Items: spans}, rows.Err()
}

// SpendByDimension aggregates cost by the given dimension.
func (p *PostgresSpanStore) SpendByDimension(ctx context.Context, groupBy string, filter *runtimemodel.SpanFilter) (map[string]money.Amount, error) {
	col, ok := allowedGroupBy[groupBy]
	if !ok {
		return nil, fmt.Errorf("invalid groupBy %q", groupBy)
	}

	query := fmt.Sprintf(`
		SELECT COALESCE(%s, 'unknown'), COALESCE(SUM(cost_amount_nanos), 0)
		FROM spans WHERE 1=1
	`, col)
	var args []any
	argNum := 1

	if filter.ID != "" {
		query += fmt.Sprintf(` AND id = $%d`, argNum)
		args = append(args, filter.ID)
		argNum++
	}
	if filter.RunID != "" {
		query += fmt.Sprintf(` AND run_id = $%d`, argNum)
		args = append(args, filter.RunID)
		argNum++
	}
	if filter.FromMs != nil {
		query += fmt.Sprintf(` AND started_at >= $%d`, argNum)
		args = append(args, pgTime(*filter.FromMs))
		argNum++
	}
	if filter.ToMs != nil {
		query += fmt.Sprintf(` AND started_at <= $%d`, argNum)
		args = append(args, pgTime(*filter.ToMs))
		argNum++
	}
	query += fmt.Sprintf(` GROUP BY %s ORDER BY SUM(cost_amount_nanos) DESC`, col)

	rows, err := p.pool.Query(ctx, query, args...)
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

func pgTime(ms int64) time.Time {
	return time.UnixMilli(ms)
}

func ptrPgTime(ms *int64) *time.Time {
	if ms == nil {
		return nil
	}
	t := time.UnixMilli(*ms)
	return &t
}

func derefBytesP(b *[]byte) []byte {
	if b == nil {
		return nil
	}
	return *b
}

func setPtrInt32(dst **int, src *int32) {
	if src != nil {
		v := int(*src)
		*dst = &v
	}
}

// Allowlist for groupBy values in SpendByDimension.
var allowedGroupBy = map[string]string{
	"model":     "CAST(model AS TEXT)",
	"run_id":    "CAST(run_id AS TEXT)",
	"connector": "CAST(connector AS TEXT)",
	"hour":      "DATE_TRUNC('hour', started_at)::TEXT",
	"day":       "DATE_TRUNC('day', started_at)::TEXT",
}
