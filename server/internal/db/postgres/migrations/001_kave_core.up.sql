-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgvector";

-- Helper function to set updated_at timestamp
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- workspaces — multi-tenancy root
CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workspaces_slug ON workspaces(slug);

CREATE TRIGGER workspaces_set_updated_at
    BEFORE UPDATE ON workspaces
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- agents — agent identities
CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    policy_id UUID,
    monthly_budget_amount_nanos BIGINT DEFAULT 100000000000,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agents_workspace_id ON agents(workspace_id);
CREATE INDEX idx_agents_policy_id ON agents(policy_id);

CREATE TRIGGER agents_set_updated_at
    BEFORE UPDATE ON agents
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- policies — what agents are allowed to do
CREATE TABLE policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    allowed_connectors TEXT[] DEFAULT ARRAY['*'],
    allowed_methods TEXT[] DEFAULT ARRAY['*'],
    budget_cap_amount_nanos BIGINT NOT NULL DEFAULT 1000000000000,
    config JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policies_workspace_id ON policies(workspace_id);

CREATE TRIGGER policies_set_updated_at
    BEFORE UPDATE ON policies
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- runs — agent task executions
CREATE TABLE runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    policy_id UUID REFERENCES policies(id) ON DELETE SET NULL,
    name TEXT,
    status TEXT NOT NULL DEFAULT 'active', -- active | completed | failed
    budget_cap_amount_nanos BIGINT,
    spent_amount_nanos BIGINT DEFAULT 0,
    metadata JSONB DEFAULT '{}',
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_runs_workspace_id ON runs(workspace_id);
CREATE INDEX idx_runs_agent_id ON runs(agent_id);
CREATE INDEX idx_runs_status ON runs(status);
CREATE INDEX idx_runs_started_at ON runs(started_at DESC);

CREATE TRIGGER runs_set_updated_at
    BEFORE UPDATE ON runs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- actions — individual agent actions within a run
CREATE TABLE actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    action_type TEXT NOT NULL, -- llm_call | tool_use | memory_read | memory_write
    connector TEXT NOT NULL,
    method TEXT NOT NULL,
    input JSONB NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_actions_run_id ON actions(run_id);
CREATE INDEX idx_actions_type ON actions(action_type);
CREATE INDEX idx_actions_connector ON actions(connector);
CREATE INDEX idx_actions_created_at ON actions(created_at DESC);

-- spans — trace records (OTel compatible)
CREATE TABLE spans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    action_id UUID NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
    parent_id UUID REFERENCES spans(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ,
    duration_ms BIGINT,
    input JSONB,
    output JSONB,
    error TEXT,
    input_tokens INTEGER,
    output_tokens INTEGER,
    cache_read_tokens INTEGER,
    cache_write_tokens INTEGER,
    model TEXT,
    cost_amount_nanos BIGINT,
    tags JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_spans_run_id ON spans(run_id);
CREATE INDEX idx_spans_action_id ON spans(action_id);
CREATE INDEX idx_spans_parent_id ON spans(parent_id);
CREATE INDEX idx_spans_started_at ON spans(started_at DESC);
CREATE INDEX idx_spans_has_error ON spans((error IS NOT NULL));

-- budget_ledger — immutable append-only cost records
CREATE TABLE budget_ledger (
    id BIGSERIAL PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    run_id UUID REFERENCES runs(id) ON DELETE SET NULL,
    action_id UUID REFERENCES actions(id) ON DELETE SET NULL,
    span_id UUID REFERENCES spans(id) ON DELETE SET NULL,
    connector TEXT NOT NULL,
    model TEXT,
    input_tokens INTEGER,
    output_tokens INTEGER,
    cache_read_tokens INTEGER,
    cache_write_tokens INTEGER,
    cost_amount_nanos BIGINT NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_budget_ledger_workspace_id ON budget_ledger(workspace_id);
CREATE INDEX idx_budget_ledger_agent_id ON budget_ledger(agent_id);
CREATE INDEX idx_budget_ledger_run_id ON budget_ledger(run_id);
CREATE INDEX idx_budget_ledger_action_id ON budget_ledger(action_id);
CREATE INDEX idx_budget_ledger_created_at ON budget_ledger(created_at DESC);
CREATE INDEX idx_budget_ledger_connector ON budget_ledger(connector);

-- No trigger on budget_ledger; it is immutable (append-only)
