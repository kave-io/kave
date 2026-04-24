ALTER TABLE spans
    ADD COLUMN IF NOT EXISTS project_id  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS env_id      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS agent_id    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS kind        TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS connector   TEXT NOT NULL DEFAULT '';

UPDATE spans s
SET project_id = r.project_id,
    env_id = r.env_id,
    agent_id = r.agent_id
FROM runs r
WHERE s.run_id = r.id
  AND s.project_id = '';

UPDATE spans s
SET kind = a.action_type,
    source = COALESCE(NULLIF(s.source, ''), 'intercepted'),
    connector = a.connector
FROM actions a
WHERE s.action_id = a.id
  AND s.connector = '';

UPDATE spans
SET trace_id = run_id
WHERE trace_id = '' OR trace_id IS NULL;

UPDATE spans
SET root_span_id = id
WHERE root_span_id = '' OR root_span_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_spans_trace_started_at ON spans(trace_id, started_at DESC)
    WHERE trace_id <> '';

CREATE INDEX IF NOT EXISTS idx_spans_project_started_at ON spans(project_id, started_at DESC)
    WHERE project_id <> '';
