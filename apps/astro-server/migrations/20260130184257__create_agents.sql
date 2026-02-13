-- Create agents and agent_versions tables
--
-- Rollback: DROP INDEX idx_versions_agent; DROP TABLE agent_versions; DROP TABLE agents;

CREATE TABLE IF NOT EXISTS agents (
    name TEXT NOT NULL PRIMARY KEY,
    registry TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_versions (
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    spec_json TEXT NOT NULL,
    published_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    PRIMARY KEY (name, version),
    FOREIGN KEY (name) REFERENCES agents(name) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_versions_agent ON agent_versions(name);
