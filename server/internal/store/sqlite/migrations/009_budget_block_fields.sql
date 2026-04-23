-- Promote the free-form budget_ledger.metadata blob into typed block_* columns.
-- Block rows are zero-cost ledger entries recording a policy/budget denial.
ALTER TABLE budget_ledger ADD COLUMN blocked INTEGER NOT NULL DEFAULT 0;
ALTER TABLE budget_ledger ADD COLUMN block_reason TEXT;
ALTER TABLE budget_ledger ADD COLUMN block_period TEXT;
CREATE INDEX IF NOT EXISTS idx_budget_ledger_blocked ON budget_ledger(blocked) WHERE blocked = 1;
