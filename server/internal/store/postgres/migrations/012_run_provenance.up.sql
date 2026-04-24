ALTER TABLE runs
    ADD COLUMN IF NOT EXISTS trigger_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trigger_id TEXT,
    ADD COLUMN IF NOT EXISTS correlation_id TEXT,
    ADD COLUMN IF NOT EXISTS session_id TEXT,
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_idempotency
    ON runs(env_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
