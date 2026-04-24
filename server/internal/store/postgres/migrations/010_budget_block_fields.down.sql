DROP INDEX IF EXISTS idx_budget_ledger_blocked;
ALTER TABLE budget_ledger
    DROP COLUMN IF EXISTS blocked,
    DROP COLUMN IF EXISTS block_reason,
    DROP COLUMN IF EXISTS block_period;
