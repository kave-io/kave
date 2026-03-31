package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kave-io/kave/core/store"
)

// PostgresSpanStore implements store.SpanStore using Postgres.
// Unlike DuckDB, Postgres handles concurrent writes natively so no buffering is needed.
type PostgresSpanStore struct {
	pool *pgxpool.Pool
}

// New creates a new Postgres span store.
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

// WriteSpan inserts a new span.
func (p *PostgresSpanStore) WriteSpan(ctx context.Context, span *store.SpanRow) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO spans (id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms, input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, model, cost_usd, tags, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`, span.ID, span.RunID, span.ActionID, span.ParentID, span.Name, pgTime(span.StartedAt), ptrPgTime(span.EndedAt), span.DurationMs, span.Input, span.Output, span.Error, span.InputTokens, span.OutputTokens, span.CacheReadTokens, span.CacheWriteTokens, span.Model, span.CostUSD, span.Tags, time.Now())
	return err
}

// UpdateSpan updates an existing span.
func (p *PostgresSpanStore) UpdateSpan(ctx context.Context, span *store.SpanRow) error {
	_, err := p.pool.Exec(ctx, `
		UPDATE spans
		SET ended_at = COALESCE($2, ended_at),
		    duration_ms = COALESCE($3, duration_ms),
		    output = COALESCE($4, output),
		    error = COALESCE($5, error),
		    input_tokens = COALESCE($6, input_tokens),
		    output_tokens = COALESCE($7, output_tokens),
		    cache_read_tokens = COALESCE($8, cache_read_tokens),
		    cache_write_tokens = COALESCE($9, cache_write_tokens),
		    model = COALESCE($10, model),
		    cost_usd = COALESCE($11, cost_usd),
		    tags = COALESCE($12, tags)
		WHERE id = $1
	`, span.ID, ptrPgTime(span.EndedAt), span.DurationMs, span.Output, span.Error, span.InputTokens, span.OutputTokens, span.CacheReadTokens, span.CacheWriteTokens, span.Model, span.CostUSD, span.Tags)
	return err
}

// GetSpan retrieves a single span by ID.
func (p *PostgresSpanStore) GetSpan(ctx context.Context, spanID string) (*store.SpanRow, error) {
	var row store.SpanRow
	var startedAt time.Time
	var endedAt *time.Time

	err := p.pool.QueryRow(ctx, `
		SELECT id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms,
		       input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, model, cost_usd, tags
		FROM spans WHERE id = $1
	`, spanID).Scan(
		&row.ID, &row.RunID, &row.ActionID, &row.ParentID, &row.Name, &startedAt, &endedAt, &row.DurationMs,
		&row.Input, &row.Output, &row.Error, &row.InputTokens, &row.OutputTokens, &row.CacheReadTokens, &row.CacheWriteTokens, &row.Model, &row.CostUSD, &row.Tags,
	)

	if err != nil {
		return nil, err
	}

	row.StartedAt = startedAt.UnixMilli()
	if endedAt != nil {
		ms := endedAt.UnixMilli()
		row.EndedAt = &ms
	}
	row.CreatedAt = time.Now().UnixMilli() // placeholder

	return &row, nil
}

// QuerySpans retrieves spans matching a filter.
func (p *PostgresSpanStore) QuerySpans(ctx context.Context, filter *store.SpanFilter) ([]*store.SpanRow, error) {
	query := `
		SELECT id, run_id, action_id, parent_id, name, started_at, ended_at, duration_ms,
		       input, output, error, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, model, cost_usd, tags
		FROM spans WHERE 1=1
	`
	var args []any
	argNum := 1

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

	query += ` ORDER BY started_at DESC`
	if filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT $%d`, argNum)
		args = append(args, filter.Limit)
	}

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spans []*store.SpanRow
	for rows.Next() {
		var row store.SpanRow
		var startedAt time.Time
		var endedAt *time.Time
		if err := rows.Scan(
			&row.ID, &row.RunID, &row.ActionID, &row.ParentID, &row.Name, &startedAt, &endedAt, &row.DurationMs,
			&row.Input, &row.Output, &row.Error, &row.InputTokens, &row.OutputTokens, &row.CacheReadTokens, &row.CacheWriteTokens, &row.Model, &row.CostUSD, &row.Tags,
		); err != nil {
			return nil, err
		}
		row.StartedAt = startedAt.UnixMilli()
		if endedAt != nil {
			ms := endedAt.UnixMilli()
			row.EndedAt = &ms
		}
		spans = append(spans, &row)
	}

	return spans, rows.Err()
}

// SpendByDimension aggregates cost by the given dimension.
func (p *PostgresSpanStore) SpendByDimension(ctx context.Context, groupBy string, filter *store.SpanFilter) (map[string]float64, error) {
	// Validate groupBy against allowlist
	col, ok := allowedGroupBy[groupBy]
	if !ok {
		return nil, fmt.Errorf("invalid groupBy %q", groupBy)
	}

	query := fmt.Sprintf(`
		SELECT COALESCE(%s, 'unknown'), COALESCE(SUM(cost_usd), 0)
		FROM spans WHERE 1=1
	`, col)
	var args []any
	argNum := 1

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

	query += fmt.Sprintf(` GROUP BY %s ORDER BY SUM(cost_usd) DESC`, col)

	rows, err := p.pool.Query(ctx, query, args...)
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

// Helper to convert UnixMilli int64 to time.Time
func pgTime(ms int64) time.Time {
	return time.UnixMilli(ms)
}

// Helper to convert *int64 to *time.Time
func ptrPgTime(ms *int64) *time.Time {
	if ms == nil {
		return nil
	}
	t := time.UnixMilli(*ms)
	return &t
}

// Allowlist for groupBy values in SpendByDimension
var allowedGroupBy = map[string]string{
	"model":     "CAST(model AS TEXT)",
	"run_id":    "CAST(run_id AS TEXT)",
	"connector": "CAST(connector AS TEXT)",
	"hour":      "DATE_TRUNC('hour', started_at)::TEXT",
	"day":       "DATE_TRUNC('day', started_at)::TEXT",
}
