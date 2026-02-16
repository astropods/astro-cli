-- Create accounts system and scope agents/agent_versions to accounts
-- Also renames version -> build_id and adds readme + validation_warnings columns
--
-- Rollback:
--   ALTER TABLE agent_versions DROP COLUMN validation_warnings;
--   ALTER TABLE agent_versions RENAME COLUMN build_id TO version;
--   ALTER TABLE agent_versions DROP COLUMN readme;
--   ALTER TABLE agent_versions DROP CONSTRAINT agent_versions_pkey;
--   ALTER TABLE agent_versions DROP CONSTRAINT agent_versions_account_id_name_fkey;
--   ALTER TABLE agent_versions DROP COLUMN account_id;
--   ALTER TABLE agent_versions ADD PRIMARY KEY (name, version);
--   DROP INDEX IF EXISTS idx_versions_agent;
--   CREATE INDEX idx_versions_agent ON agent_versions(name);
--   ALTER TABLE agents DROP CONSTRAINT agents_pkey;
--   ALTER TABLE agents DROP CONSTRAINT agents_account_id_fkey;
--   ALTER TABLE agents DROP COLUMN account_id;
--   ALTER TABLE agents ADD PRIMARY KEY (name);
--   DROP INDEX IF EXISTS idx_account_members_user;
--   DROP TABLE IF EXISTS account_members;
--   DROP TABLE IF EXISTS accounts;

-- 1. Create accounts and account_members
CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(39) NOT NULL UNIQUE,
    type VARCHAR(20) NOT NULL DEFAULT 'personal',
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS account_members (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'owner',
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_account_members_user ON account_members(user_id);

-- 2. Drop agent_versions FK before changing agents PK (IF EXISTS for idempotency)
ALTER TABLE agent_versions DROP CONSTRAINT IF EXISTS agent_versions_name_fkey;

-- 3. Add account_id to agents and migrate PK
ALTER TABLE agents ADD COLUMN account_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE agents ALTER COLUMN account_id DROP DEFAULT;
ALTER TABLE agents ADD CONSTRAINT agents_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;
ALTER TABLE agents DROP CONSTRAINT agents_pkey;
ALTER TABLE agents ADD PRIMARY KEY (account_id, name);

-- 4. Add account_id to agent_versions and migrate PK
ALTER TABLE agent_versions ADD COLUMN account_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';
ALTER TABLE agent_versions ALTER COLUMN account_id DROP DEFAULT;
ALTER TABLE agent_versions DROP CONSTRAINT agent_versions_pkey;
ALTER TABLE agent_versions ADD PRIMARY KEY (account_id, name, version);
ALTER TABLE agent_versions ADD CONSTRAINT agent_versions_account_id_name_fkey
    FOREIGN KEY (account_id, name) REFERENCES agents(account_id, name) ON DELETE CASCADE;

-- 5. Rebuild index on composite key
DROP INDEX IF EXISTS idx_versions_agent;
CREATE INDEX idx_versions_agent ON agent_versions(account_id, name);

-- 6. Rename version -> build_id
ALTER TABLE agent_versions RENAME COLUMN version TO build_id;

-- 7. Add readme and validation_warnings columns
ALTER TABLE agent_versions ADD COLUMN IF NOT EXISTS readme TEXT NOT NULL DEFAULT '';
ALTER TABLE agent_versions ADD COLUMN validation_warnings TEXT NOT NULL DEFAULT '';
