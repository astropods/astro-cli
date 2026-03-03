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

CREATE TABLE public.account_members (
    account_id uuid NOT NULL,
    user_id text NOT NULL,
    role varchar(20) NOT NULL DEFAULT 'owner',
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT account_members_pkey PRIMARY KEY (account_id, user_id),
    CONSTRAINT account_members_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_account_members_user ON public.account_members(user_id);

CREATE TABLE public.agents (
    account_id uuid NOT NULL,
    name text NOT NULL,
    registry text NOT NULL,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    CONSTRAINT agents_pkey PRIMARY KEY (account_id, name),
    CONSTRAINT agents_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

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

CREATE TABLE public.agent_published_versions (
    account_id uuid NOT NULL,
    name text NOT NULL,
    version text NOT NULL,
    build_id text NOT NULL,
    published_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT agent_published_versions_pkey PRIMARY KEY (account_id, name, version),
    CONSTRAINT agent_published_versions_account_id_name_build_id_key UNIQUE (account_id, name, build_id),
    CONSTRAINT agent_published_versions_account_id_name_build_id_fkey FOREIGN KEY (account_id, name, build_id) REFERENCES public.agent_versions(account_id, name, build_id) ON DELETE CASCADE
);

CREATE INDEX idx_published_versions_agent ON public.agent_published_versions(account_id, name);

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
