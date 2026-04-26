-- ── Audit Logs ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS audit_logs (
    id            TEXT PRIMARY KEY,
    org_id        TEXT NOT NULL,
    project_id    TEXT,
    env_id        TEXT,
    actor_id      TEXT NOT NULL,
    actor_type    TEXT NOT NULL DEFAULT 'system',
    event         TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    diff_before   BYTEA,
    diff_after    BYTEA,
    ip            TEXT,
    provenance    BYTEA,
    created_at    BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_org_id ON audit_logs(org_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_project_id ON audit_logs(project_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_env_id ON audit_logs(env_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_id ON audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource_type ON audit_logs(resource_type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);
