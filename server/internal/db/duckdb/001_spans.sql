CREATE TABLE IF NOT EXISTS spans (
    id                  VARCHAR PRIMARY KEY,
    run_id              VARCHAR NOT NULL,
    action_id           VARCHAR NOT NULL,
    parent_id           VARCHAR,
    name                VARCHAR NOT NULL,
    started_at          BIGINT NOT NULL,
    ended_at            BIGINT,
    duration_ms         BIGINT,
    input               VARCHAR,
    output              VARCHAR,
    error               VARCHAR,
    input_tokens        INTEGER,
    output_tokens       INTEGER,
    cache_read_tokens   INTEGER,
    cache_write_tokens  INTEGER,
    model               VARCHAR,
    cost_amount_nanos   BIGINT,
    tags                VARCHAR,
    created_at          BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_spans_run_id    ON spans(run_id);
CREATE INDEX IF NOT EXISTS idx_spans_action_id ON spans(action_id);
CREATE INDEX IF NOT EXISTS idx_spans_started_at ON spans(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_spans_model ON spans(model);

-- Cost summary by day and model
-- Note: started_at is a BIGINT (milliseconds since epoch)
-- DuckDB date_trunc expects a timestamp; convert with to_timestamp(ms / 1000.0)
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
