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
ALTER TABLE spans ADD COLUMN IF NOT EXISTS trace_id     VARCHAR NOT NULL DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS root_span_id VARCHAR NOT NULL DEFAULT '';

-- Gap 20: validation metadata
ALTER TABLE spans ADD COLUMN IF NOT EXISTS validation_meta VARCHAR;
