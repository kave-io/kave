-- Gap 15/16: extended token + non-token usage columns
ALTER TABLE spans
    ADD COLUMN IF NOT EXISTS reasoning_tokens    INTEGER,
    ADD COLUMN IF NOT EXISTS audio_input_tokens  INTEGER,
    ADD COLUMN IF NOT EXISTS audio_output_tokens INTEGER,
    ADD COLUMN IF NOT EXISTS image_units         INTEGER,
    ADD COLUMN IF NOT EXISTS request_count       INTEGER,
    ADD COLUMN IF NOT EXISTS compute_ms          BIGINT,
    ADD COLUMN IF NOT EXISTS storage_bytes       BIGINT,
    ADD COLUMN IF NOT EXISTS bandwidth_bytes     BIGINT,
    ADD COLUMN IF NOT EXISTS usage_detail        JSONB;

ALTER TABLE budget_ledger
    ADD COLUMN IF NOT EXISTS reasoning_tokens    INTEGER,
    ADD COLUMN IF NOT EXISTS audio_input_tokens  INTEGER,
    ADD COLUMN IF NOT EXISTS audio_output_tokens INTEGER,
    ADD COLUMN IF NOT EXISTS image_units         INTEGER,
    ADD COLUMN IF NOT EXISTS request_count       INTEGER,
    ADD COLUMN IF NOT EXISTS compute_ms          BIGINT,
    ADD COLUMN IF NOT EXISTS storage_bytes       BIGINT,
    ADD COLUMN IF NOT EXISTS bandwidth_bytes     BIGINT,
    ADD COLUMN IF NOT EXISTS usage_detail        JSONB;

-- Gap 17: PriceBook non-token pricing + temporal versioning
ALTER TABLE price_book_entries
    ADD COLUMN IF NOT EXISTS currency                           TEXT NOT NULL DEFAULT 'USD',
    ADD COLUMN IF NOT EXISTS reasoning_per_million_amount_nanos BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS audio_input_per_million_amount_nanos  BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS audio_output_per_million_amount_nanos BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS image_unit_price_amount_nanos         BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS per_request_amount_nanos              BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS per_compute_ms_amount_nanos           BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS per_gb_stored_amount_nanos            BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS per_gb_transferred_amount_nanos       BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS effective_from           BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS effective_to             BIGINT,
    ADD COLUMN IF NOT EXISTS revision_note            TEXT;

CREATE INDEX IF NOT EXISTS idx_price_book_entries_effective
    ON price_book_entries(effective_from, effective_to);

-- Gap 18: rename tags → attrs
ALTER TABLE spans RENAME COLUMN tags TO attrs;

-- Gap 19: TraceID / RootSpanID (TEXT NOT NULL with empty-string default)
ALTER TABLE spans
    ADD COLUMN IF NOT EXISTS trace_id     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS root_span_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_spans_trace_id ON spans(trace_id)
    WHERE trace_id <> '';

-- Gap 20: validation metadata
ALTER TABLE spans
    ADD COLUMN IF NOT EXISTS validation_meta JSONB;
