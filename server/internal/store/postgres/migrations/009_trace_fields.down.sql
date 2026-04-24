DROP INDEX IF EXISTS idx_spans_project_started_at;
DROP INDEX IF EXISTS idx_spans_trace_started_at;

ALTER TABLE spans
    DROP COLUMN IF EXISTS connector,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS kind,
    DROP COLUMN IF EXISTS agent_id,
    DROP COLUMN IF EXISTS env_id,
    DROP COLUMN IF EXISTS project_id;
