ALTER TABLE spans ADD COLUMN IF NOT EXISTS project_id VARCHAR DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS env_id VARCHAR DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS agent_id VARCHAR DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS kind VARCHAR DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS source VARCHAR DEFAULT '';
ALTER TABLE spans ADD COLUMN IF NOT EXISTS connector VARCHAR DEFAULT '';

UPDATE spans
SET kind = COALESCE(NULLIF(kind, ''), 'action'),
    source = COALESCE(NULLIF(source, ''), 'intercepted')
WHERE COALESCE(kind, '') = '' OR COALESCE(source, '') = '';

UPDATE spans
SET trace_id = run_id
WHERE COALESCE(trace_id, '') = '';

UPDATE spans
SET root_span_id = id
WHERE COALESCE(root_span_id, '') = '';

CREATE INDEX IF NOT EXISTS idx_spans_trace_started_at ON spans(trace_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_spans_project_started_at ON spans(project_id, started_at DESC);
