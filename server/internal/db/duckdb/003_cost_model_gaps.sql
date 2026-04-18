-- Drop view and indexes before altering the table
DROP VIEW IF EXISTS spans_cost_summary;
DROP INDEX IF EXISTS idx_spans_run_id;
DROP INDEX IF EXISTS idx_spans_action_id;
DROP INDEX IF EXISTS idx_spans_started_at;
DROP INDEX IF EXISTS idx_spans_model;

-- Gap 15/16: extended token + non-token usage columns
ALTER TABLE spans ADD COLUMN IF NOT EXISTS reasoning_tokens    INTEGER;
ALTER TABLE spans ADD COLUMN IF NOT EXISTS audio_input_tokens  INTEGER;
ALTER TABLE spans ADD COLUMN IF NOT EXISTS audio_output_tokens INTEGER;
ALTER TABLE spans ADD COLUMN IF NOT EXISTS image_units         INTEGER;
ALTER TABLE spans ADD COLUMN IF NOT EXISTS request_count       INTEGER;
ALTER TABLE spans ADD COLUMN IF NOT EXISTS compute_ms          BIGINT;
ALTER TABLE spans ADD COLUMN IF NOT EXISTS storage_bytes       BIGINT;
ALTER TABLE spans ADD COLUMN IF NOT EXISTS bandwidth_bytes     BIGINT;
ALTER TABLE spans ADD COLUMN IF NOT EXISTS usage_detail        VARCHAR;

-- Gap 18: rename tags → attrs
ALTER TABLE spans RENAME COLUMN tags TO attrs;

-- Gap 19: TraceID / RootSpanID
ALTER TABLE spans ADD COLUMN IF NOT EXISTS trace_id     VARCHAR DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS root_span_id VARCHAR DEFAULT '';

-- Gap 20: validation metadata
ALTER TABLE spans ADD COLUMN IF NOT EXISTS validation_meta VARCHAR;

-- Recreate indexes and view
CREATE INDEX IF NOT EXISTS idx_spans_run_id    ON spans(run_id);
CREATE INDEX IF NOT EXISTS idx_spans_action_id ON spans(action_id);
CREATE INDEX IF NOT EXISTS idx_spans_started_at ON spans(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_spans_model ON spans(model);

-- Cost summary by day and model
CREATE VIEW IF NOT EXISTS spans_cost_summary AS
SELECT
    cast(started_at / (1000.0 * 3600 * 24) as BIGINT) AS day_key,
    model,
    count(*)                        AS span_count,
    sum(cost_amount_nanos)          AS total_cost_amount_nanos,
    sum(input_tokens)               AS total_input_tokens,
    sum(output_tokens)              AS total_output_tokens,
    avg(duration_ms)                AS avg_duration_ms
FROM spans
GROUP BY 1, 2;
