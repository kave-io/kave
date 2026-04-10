CREATE TABLE IF NOT EXISTS price_book_entries (
    id                      TEXT PRIMARY KEY,
    version                 TEXT NOT NULL,
    provider                TEXT NOT NULL,
    match                   TEXT NOT NULL,
    source                  TEXT NOT NULL,
    input_per_million       REAL NOT NULL,
    output_per_million      REAL NOT NULL,
    cache_read_per_million  REAL NOT NULL DEFAULT 0.0,
    cache_write_per_million REAL NOT NULL DEFAULT 0.0,
    sort_order              INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_price_book_entries_sort_order ON price_book_entries(sort_order);

