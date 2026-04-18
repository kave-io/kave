CREATE TABLE IF NOT EXISTS fx_rates (
    base_currency   TEXT NOT NULL,
    quote_currency  TEXT NOT NULL,
    rate            TEXT NOT NULL,
    provider        TEXT NOT NULL,
    as_of_date      TEXT NOT NULL,
    fetched_at      INTEGER NOT NULL,
    PRIMARY KEY (base_currency, quote_currency)
);

CREATE INDEX IF NOT EXISTS idx_fx_rates_base ON fx_rates(base_currency);

CREATE TABLE IF NOT EXISTS fx_currencies (
    code        TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    symbol      TEXT NOT NULL DEFAULT '',
    fetched_at  INTEGER NOT NULL
);
