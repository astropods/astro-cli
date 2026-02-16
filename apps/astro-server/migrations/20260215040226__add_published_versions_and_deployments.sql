-- Create agent_published_versions and deployments tables
--
-- Rollback:
--   DROP INDEX IF EXISTS idx_deployments_account_agent;
--   DROP INDEX IF EXISTS idx_deployments_active_agent;
--   DROP TABLE IF EXISTS deployments;
--   DROP INDEX IF EXISTS idx_published_versions_agent;
--   DROP TABLE IF EXISTS agent_published_versions;

CREATE TABLE agent_published_versions (
    account_id UUID NOT NULL,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    build_id TEXT NOT NULL,
    published_at TIMESTAMP NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, name, version),
    FOREIGN KEY (account_id, name, build_id) REFERENCES agent_versions(account_id, name, build_id) ON DELETE CASCADE,
    UNIQUE (account_id, name, build_id)
);

CREATE INDEX idx_published_versions_agent ON agent_published_versions(account_id, name);

CREATE TABLE deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id),
    agent_name VARCHAR NOT NULL,
    build_id VARCHAR NOT NULL,
    namespace VARCHAR NOT NULL,
    deployment_spec_json TEXT NOT NULL,
    status VARCHAR NOT NULL DEFAULT 'active',
    deployed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    undeployed_at TIMESTAMP
);

CREATE UNIQUE INDEX idx_deployments_active_agent
    ON deployments (account_id, agent_name)
    WHERE status = 'active';

CREATE INDEX idx_deployments_account_agent
    ON deployments (account_id, agent_name);
