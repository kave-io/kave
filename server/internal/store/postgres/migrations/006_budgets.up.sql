-- budgets — one record per agent
CREATE TABLE budgets (
    id           TEXT PRIMARY KEY,
    agent_id     TEXT NOT NULL UNIQUE REFERENCES agents(id) ON DELETE CASCADE,
    hard_cap_nanos BIGINT NOT NULL,
    soft_cap_nanos BIGINT NOT NULL DEFAULT 0,
    period       TEXT NOT NULL DEFAULT 'run',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_budgets_agent_id ON budgets(agent_id);
