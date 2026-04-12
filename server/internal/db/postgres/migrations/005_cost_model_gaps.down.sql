-- Rollback Gap 20: validation metadata
ALTER TABLE spans DROP COLUMN IF EXISTS validation_meta;

-- Rollback Gap 19: TraceID / RootSpanID
DROP INDEX IF EXISTS idx_spans_trace_id;
ALTER TABLE spans DROP COLUMN IF EXISTS trace_id;
ALTER TABLE spans DROP COLUMN IF EXISTS root_span_id;

-- Rollback Gap 18: rename attrs → tags
ALTER TABLE spans RENAME COLUMN attrs TO tags;

-- Rollback Gap 17: PriceBook temporal versioning + non-token pricing
DROP INDEX IF EXISTS idx_price_book_entries_effective;
ALTER TABLE price_book_entries
    DROP COLUMN IF EXISTS reasoning_per_million,
    DROP COLUMN IF EXISTS audio_input_per_million,
    DROP COLUMN IF EXISTS audio_output_per_million,
    DROP COLUMN IF EXISTS image_unit_price,
    DROP COLUMN IF EXISTS per_request,
    DROP COLUMN IF EXISTS per_compute_ms,
    DROP COLUMN IF EXISTS per_gb_stored,
    DROP COLUMN IF EXISTS per_gb_transferred,
    DROP COLUMN IF EXISTS effective_from,
    DROP COLUMN IF EXISTS effective_to,
    DROP COLUMN IF EXISTS revision_note;

-- Rollback Gap 15/16: extended usage columns
ALTER TABLE budget_ledger
    DROP COLUMN IF EXISTS reasoning_tokens,
    DROP COLUMN IF EXISTS audio_input_tokens,
    DROP COLUMN IF EXISTS audio_output_tokens,
    DROP COLUMN IF EXISTS image_units,
    DROP COLUMN IF EXISTS request_count,
    DROP COLUMN IF EXISTS compute_ms,
    DROP COLUMN IF EXISTS storage_bytes,
    DROP COLUMN IF EXISTS bandwidth_bytes,
    DROP COLUMN IF EXISTS usage_detail;

ALTER TABLE spans
    DROP COLUMN IF EXISTS reasoning_tokens,
    DROP COLUMN IF EXISTS audio_input_tokens,
    DROP COLUMN IF EXISTS audio_output_tokens,
    DROP COLUMN IF EXISTS image_units,
    DROP COLUMN IF EXISTS request_count,
    DROP COLUMN IF EXISTS compute_ms,
    DROP COLUMN IF EXISTS storage_bytes,
    DROP COLUMN IF EXISTS bandwidth_bytes,
    DROP COLUMN IF EXISTS usage_detail;
