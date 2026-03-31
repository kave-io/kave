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
    cost_usd            DOUBLE,
    tags                VARCHAR,
    created_at          BIGINT NOT NULL DEFAULT extract(epoch from now()) * 1000
);

CREATE INDEX IF NOT EXISTS idx_spans_run_id    ON spans(run_id);
CREATE INDEX IF NOT EXISTS idx_spans_action_id ON spans(action_id);
CREATE INDEX IF NOT EXISTS idx_spans_started_at ON spans(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_spans_model ON spans(model);

CREATE VIEW IF NOT EXISTS spans_cost_summary AS
SELECT
    date_trunc('day', to_timestamp(started_at / 1000.0))  AS day,
    model,
    count(*)                        AS span_count,
    sum(cost_usd)                   AS total_cost_usd,
    sum(input_tokens)               AS total_input_tokens,
    sum(output_tokens)              AS total_output_tokens,
    avg(duration_ms)                AS avg_duration_ms
FROM spans
GROUP BY 1, 2;
