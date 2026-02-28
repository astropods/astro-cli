-- Astro declarative schema (SDL)
-- This file is the single source of truth for the database schema.
-- Atlas diffs this against the live DB and applies only what changed.

CREATE TABLE accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(39) NOT NULL UNIQUE,
    type varchar(20) NOT NULL DEFAULT 'personal',
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now()
);

CREATE TABLE account_members (
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id text NOT NULL,
    role varchar(20) NOT NULL DEFAULT 'owner',
    created_at timestamp NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, user_id)
);

CREATE INDEX idx_account_members_user ON account_members(user_id);

CREATE TABLE agents (
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name text NOT NULL,
    registry text NOT NULL,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    PRIMARY KEY (account_id, name)
);

CREATE TABLE agent_versions (
    account_id uuid NOT NULL,
    name text NOT NULL,
    build_id text NOT NULL,
    spec_json text NOT NULL,
    readme text NOT NULL DEFAULT '',
    validation_warnings text NOT NULL DEFAULT '',
    published_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    PRIMARY KEY (account_id, name, build_id),
    FOREIGN KEY (account_id, name) REFERENCES agents(account_id, name) ON DELETE CASCADE
);

CREATE INDEX idx_versions_agent ON agent_versions(account_id, name);

CREATE TABLE agent_published_versions (
    account_id uuid NOT NULL,
    name text NOT NULL,
    version text NOT NULL,
    build_id text NOT NULL,
    published_at timestamp NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, name, version),
    UNIQUE (account_id, name, build_id),
    FOREIGN KEY (account_id, name, build_id) REFERENCES agent_versions(account_id, name, build_id) ON DELETE CASCADE
);

CREATE INDEX idx_published_versions_agent ON agent_published_versions(account_id, name);

CREATE TABLE deployments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES accounts(id),
    agent_name varchar NOT NULL,
    build_id varchar NOT NULL,
    namespace varchar NOT NULL,
    deployment_spec_json text NOT NULL,
    status varchar NOT NULL DEFAULT 'active',
    deployed_at timestamp NOT NULL DEFAULT now(),
    undeployed_at timestamp
);

CREATE INDEX idx_deployments_account_agent ON deployments(account_id, agent_name);

CREATE UNIQUE INDEX idx_deployments_active_agent ON deployments(account_id, agent_name) WHERE status = 'active';
