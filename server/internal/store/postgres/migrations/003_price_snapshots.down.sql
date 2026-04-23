ALTER TABLE budget_ledger
    DROP COLUMN IF EXISTS price_snapshot,
    DROP COLUMN IF EXISTS price_version;

ALTER TABLE spans
    DROP COLUMN IF EXISTS price_snapshot,
    DROP COLUMN IF EXISTS price_version;
