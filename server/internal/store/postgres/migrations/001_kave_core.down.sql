-- Reverse the 001_kave_core migration

DROP TABLE IF EXISTS budget_ledger;
DROP TABLE IF EXISTS spans;
DROP TABLE IF EXISTS actions;
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS policies;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS environments;
DROP TABLE IF EXISTS workspaces;

DROP FUNCTION IF EXISTS set_updated_at();

