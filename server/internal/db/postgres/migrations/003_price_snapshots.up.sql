ALTER TABLE spans
    ADD COLUMN IF NOT EXISTS price_version TEXT,
    ADD COLUMN IF NOT EXISTS price_snapshot JSONB;

ALTER TABLE budget_ledger
    ADD COLUMN IF NOT EXISTS price_version TEXT,
    ADD COLUMN IF NOT EXISTS price_snapshot JSONB;

