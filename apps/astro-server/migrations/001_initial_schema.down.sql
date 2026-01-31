-- Migration: 001_initial_schema (down)
-- Description: Drop agents and agent_versions tables

DROP INDEX IF EXISTS idx_versions_agent;
DROP TABLE IF EXISTS agent_versions;
DROP TABLE IF EXISTS agents;
