CREATE TABLE IF NOT EXISTS price_book_entries (
    id TEXT PRIMARY KEY,
    version TEXT NOT NULL,
    provider TEXT NOT NULL,
    match TEXT NOT NULL,
    source TEXT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    input_per_million_amount_nanos BIGINT NOT NULL,
    output_per_million_amount_nanos BIGINT NOT NULL,
    cache_read_per_million_amount_nanos BIGINT NOT NULL DEFAULT 0,
    cache_write_per_million_amount_nanos BIGINT NOT NULL DEFAULT 0,
    reasoning_per_million_amount_nanos BIGINT NOT NULL DEFAULT 0,
    audio_input_per_million_amount_nanos BIGINT NOT NULL DEFAULT 0,
    audio_output_per_million_amount_nanos BIGINT NOT NULL DEFAULT 0,
    image_unit_price_amount_nanos BIGINT NOT NULL DEFAULT 0,
    per_request_amount_nanos BIGINT NOT NULL DEFAULT 0,
    per_compute_ms_amount_nanos BIGINT NOT NULL DEFAULT 0,
    per_gb_stored_amount_nanos BIGINT NOT NULL DEFAULT 0,
    per_gb_transferred_amount_nanos BIGINT NOT NULL DEFAULT 0,
    effective_from BIGINT NOT NULL DEFAULT 0,
    effective_to BIGINT,
    revision_note TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_price_book_entries_sort_order ON price_book_entries(sort_order);
