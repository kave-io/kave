-- Reverse 002_auth migration

DROP TABLE IF EXISTS credentials;
DROP TABLE IF EXISTS agent_tokens;
DROP TABLE IF EXISTS revoked_tokens;
