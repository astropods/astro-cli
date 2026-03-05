-- Astro declarative schema (SDL)
-- This file is the single source of truth for the database schema.
-- Atlas diffs this against the live DB and applies only what changed.

CREATE TABLE public.accounts (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name varchar(39) NOT NULL,
    type varchar(20) NOT NULL DEFAULT 'personal',
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT accounts_pkey PRIMARY KEY (id),
    CONSTRAINT accounts_name_key UNIQUE (name)
);

CREATE INDEX idx_accounts_name_prefix ON public.accounts(name text_pattern_ops);

CREATE TABLE public.account_organizations (
    account_id uuid NOT NULL,
    workos_org_id text NOT NULL,
    CONSTRAINT account_organizations_pkey PRIMARY KEY (account_id),
    CONSTRAINT account_organizations_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE,
    CONSTRAINT account_organizations_workos_org_id_key UNIQUE (workos_org_id)
);

CREATE TABLE public.account_members (
    account_id uuid NOT NULL,
    user_id text NOT NULL,
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT account_members_pkey PRIMARY KEY (account_id, user_id),
    CONSTRAINT account_members_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_account_members_user ON public.account_members(user_id);

CREATE TABLE public.account_member_workos (
    account_id uuid NOT NULL,
    user_id text NOT NULL,
    workos_membership_id text NOT NULL,
    CONSTRAINT account_member_workos_pkey PRIMARY KEY (account_id, user_id),
    CONSTRAINT account_member_workos_fkey FOREIGN KEY (account_id, user_id) REFERENCES public.account_members(account_id, user_id) ON DELETE CASCADE,
    CONSTRAINT account_member_workos_workos_membership_id_key UNIQUE (workos_membership_id)
);

CREATE TABLE public.agents (
    account_id uuid NOT NULL,
    name text NOT NULL,
    registry text NOT NULL,
    visibility varchar(10) NOT NULL DEFAULT 'private',
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    CONSTRAINT agents_pkey PRIMARY KEY (account_id, name),
    CONSTRAINT agents_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_agents_public ON public.agents(visibility) WHERE visibility = 'public';

CREATE TABLE public.agent_versions (
    account_id uuid NOT NULL,
    name text NOT NULL,
    build_id text NOT NULL,
    spec_json text NOT NULL,
    readme text NOT NULL DEFAULT '',
    validation_warnings text NOT NULL DEFAULT '',
    published_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    CONSTRAINT agent_versions_pkey PRIMARY KEY (account_id, name, build_id),
    CONSTRAINT agent_versions_account_id_name_fkey FOREIGN KEY (account_id, name) REFERENCES public.agents(account_id, name) ON DELETE CASCADE
);

CREATE INDEX idx_versions_agent ON public.agent_versions(account_id, name);

CREATE TABLE public.workos_event_cursor (
    id integer NOT NULL DEFAULT 1,
    cursor_id text NOT NULL DEFAULT '',
    updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT workos_event_cursor_pkey PRIMARY KEY (id),
    CONSTRAINT workos_event_cursor_singleton CHECK (id = 1)
);

CREATE TABLE public.deployments (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL,
    agent_name varchar NOT NULL,
    build_id varchar NOT NULL,
    namespace varchar NOT NULL,
    deployment_spec_json text NOT NULL,
    status varchar NOT NULL DEFAULT 'active',
    deployed_at timestamp NOT NULL DEFAULT now(),
    undeployed_at timestamp,
    CONSTRAINT deployments_pkey PRIMARY KEY (id),
    CONSTRAINT deployments_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id)
);

CREATE INDEX idx_deployments_account_agent ON public.deployments(account_id, agent_name);

CREATE UNIQUE INDEX idx_deployments_active_agent ON public.deployments(account_id, agent_name) WHERE status = 'active';

CREATE TABLE public.waitlist (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL,
    email text NOT NULL,
    invited_at timestamp,
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT waitlist_pkey PRIMARY KEY (id),
    CONSTRAINT waitlist_email_key UNIQUE (email)
);
