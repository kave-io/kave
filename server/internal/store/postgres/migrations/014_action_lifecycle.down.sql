DROP INDEX IF EXISTS idx_actions_source;
DROP INDEX IF EXISTS idx_actions_status;
DROP INDEX IF EXISTS idx_actions_agent_id;

ALTER TABLE actions
    DROP COLUMN IF EXISTS external_id,
    DROP COLUMN IF EXISTS provider_req_id,
    DROP COLUMN IF EXISTS retry_reason,
    DROP COLUMN IF EXISTS max_attempts,
    DROP COLUMN IF EXISTS attempt,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS seq,
    DROP COLUMN IF EXISTS depth,
    DROP COLUMN IF EXISTS ended_at,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS error,
    DROP COLUMN IF EXISTS output,
    DROP COLUMN IF EXISTS parent_id,
    DROP COLUMN IF EXISTS env_id,
    DROP COLUMN IF EXISTS project_id,
    DROP COLUMN IF EXISTS agent_id;
