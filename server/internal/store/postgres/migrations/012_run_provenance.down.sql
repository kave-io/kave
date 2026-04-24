DROP INDEX IF EXISTS idx_runs_idempotency;

ALTER TABLE runs
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS session_id,
    DROP COLUMN IF EXISTS correlation_id,
    DROP COLUMN IF EXISTS trigger_id,
    DROP COLUMN IF EXISTS trigger_type;
