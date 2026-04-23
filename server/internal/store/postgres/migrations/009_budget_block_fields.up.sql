ALTER TABLE budget_ledger
    ADD COLUMN IF NOT EXISTS blocked BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS block_reason TEXT,
    ADD COLUMN IF NOT EXISTS block_period TEXT;

CREATE INDEX IF NOT EXISTS idx_budget_ledger_blocked ON budget_ledger(blocked) WHERE blocked;
