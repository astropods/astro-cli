-- Astro declarative schema (SDL)
-- This file is the single source of truth for the database schema.
-- Atlas diffs this against the live DB and applies only what changed.

CREATE TABLE public.accounts (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name varchar(39) NOT NULL,
    type varchar(20) NOT NULL DEFAULT 'personal',
    openmeter_customer_id text,
    deleted_at timestamp,
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
    ecr_namespace text NOT NULL DEFAULT '',
    spec_json text NOT NULL,
    readme text NOT NULL DEFAULT '',
    agent_card_json text NOT NULL DEFAULT '',
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
    stuck_event_id text,
    stuck_since timestamp,
    updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT workos_event_cursor_pkey PRIMARY KEY (id),
    CONSTRAINT workos_event_cursor_singleton CHECK (id = 1)
);

CREATE TABLE public.workos_event_errors (
    event_id text NOT NULL,
    event_type text NOT NULL,
    event_data text NOT NULL DEFAULT '',
    error text NOT NULL,
    attempts integer NOT NULL DEFAULT 1,
    first_failed_at timestamp NOT NULL DEFAULT now(),
    last_failed_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT workos_event_errors_pkey PRIMARY KEY (event_id)
);

CREATE TABLE public.deployments (
    id varchar(11) NOT NULL,
    account_id uuid NOT NULL,
    agent_name varchar NOT NULL,
    build_id varchar NOT NULL,
    namespace varchar NOT NULL,
    display_name varchar(64) NOT NULL DEFAULT '',
    deployment_spec_json text NOT NULL,
    encrypted_data_key bytea,
    kms_key_arn varchar,
    status varchar NOT NULL DEFAULT 'active',
    deployed_at timestamp NOT NULL DEFAULT now(),
    undeployed_at timestamp,
    CONSTRAINT deployments_pkey PRIMARY KEY (id),
    CONSTRAINT deployments_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id)
);

CREATE INDEX idx_deployments_account_agent ON public.deployments(account_id, agent_name);

CREATE UNIQUE INDEX idx_deployments_active_display_name ON public.deployments(account_id, display_name) WHERE status = 'active' AND display_name != '';

-- Normalized deployment spec tables (Phase 1: dual-write alongside deployment_spec_json)

CREATE TABLE public.deployment_workloads (
    id serial NOT NULL,
    deployment_id varchar(11) NOT NULL,
    name varchar NOT NULL,
    component_kind varchar NOT NULL,
    component_key varchar NOT NULL DEFAULT '',
    workload_type varchar NOT NULL,
    image varchar NOT NULL,
    replicas int NOT NULL DEFAULT 1,
    cpu_request varchar NOT NULL DEFAULT '',
    memory_request varchar NOT NULL DEFAULT '',
    cpu_limit varchar NOT NULL DEFAULT '',
    memory_limit varchar NOT NULL DEFAULT '',
    gpu_vram varchar,
    gpu_runtime varchar,
    gpu_count int,
    update_strategy varchar,
    update_max_unavailable varchar,
    update_max_surge varchar,
    healthcheck_path varchar,
    healthcheck_port int,
    healthcheck_interval varchar,
    healthcheck_timeout varchar,
    healthcheck_retries int,
    healthcheck_test text,
    trigger_type varchar,
    trigger_schedule varchar,
    persistent boolean NOT NULL DEFAULT false,
    distributed boolean NOT NULL DEFAULT false,
    CONSTRAINT deployment_workloads_pkey PRIMARY KEY (id),
    CONSTRAINT deployment_workloads_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_deployment_workloads_name ON public.deployment_workloads(deployment_id, name);

CREATE TABLE public.deployment_services (
    id serial NOT NULL,
    workload_id int NOT NULL,
    name varchar NOT NULL,
    port int NOT NULL,
    target_port int NOT NULL,
    protocol varchar NOT NULL DEFAULT 'http',
    CONSTRAINT deployment_services_pkey PRIMARY KEY (id),
    CONSTRAINT deployment_services_workload_id_fkey FOREIGN KEY (workload_id) REFERENCES public.deployment_workloads(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_deployment_services_name ON public.deployment_services(workload_id, name);

CREATE TABLE public.deployment_ingresses (
    id serial NOT NULL,
    service_id int NOT NULL,
    hostname varchar NOT NULL,
    path varchar NOT NULL DEFAULT '/',
    tls_enabled boolean NOT NULL DEFAULT true,
    CONSTRAINT deployment_ingresses_pkey PRIMARY KEY (id),
    CONSTRAINT deployment_ingresses_service_id_fkey FOREIGN KEY (service_id) REFERENCES public.deployment_services(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_deployment_ingresses_service ON public.deployment_ingresses(service_id);

CREATE TABLE public.deployment_volumes (
    id serial NOT NULL,
    workload_id int NOT NULL,
    mount_path varchar NOT NULL,
    size varchar NOT NULL DEFAULT '10Gi',
    storage_class varchar,
    access_mode varchar NOT NULL DEFAULT 'ReadWriteOnce',
    CONSTRAINT deployment_volumes_pkey PRIMARY KEY (id),
    CONSTRAINT deployment_volumes_workload_id_fkey FOREIGN KEY (workload_id) REFERENCES public.deployment_workloads(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_deployment_volumes_path ON public.deployment_volumes(workload_id, mount_path);

CREATE TABLE public.deployment_env_vars (
    workload_id int NOT NULL,
    key varchar NOT NULL,
    value text NOT NULL DEFAULT '',
    source varchar NOT NULL DEFAULT 'direct',
    nonce bytea,
    CONSTRAINT deployment_env_vars_pkey PRIMARY KEY (workload_id, key),
    CONSTRAINT deployment_env_vars_workload_id_fkey FOREIGN KEY (workload_id) REFERENCES public.deployment_workloads(id) ON DELETE CASCADE
);

CREATE TABLE public.deployment_variables (
    deployment_id varchar(11) NOT NULL,
    name varchar NOT NULL,
    value text NOT NULL DEFAULT '',
    secret boolean NOT NULL DEFAULT false,
    optional boolean NOT NULL DEFAULT false,
    targets text[] NOT NULL DEFAULT '{}',
    nonce bytea,
    CONSTRAINT deployment_variables_pkey PRIMARY KEY (deployment_id, name),
    CONSTRAINT deployment_variables_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE TABLE public.namespace_ownership (
    namespace varchar NOT NULL,
    account_id uuid NOT NULL,
    agent_name text NOT NULL,
    deployment_id varchar(11),
    source_account text NOT NULL DEFAULT '',
    scanned_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT namespace_ownership_pkey PRIMARY KEY (namespace),
    CONSTRAINT namespace_ownership_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id),
    CONSTRAINT namespace_ownership_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id)
);

CREATE INDEX idx_namespace_ownership_account ON public.namespace_ownership(account_id);

CREATE TABLE public.connected_devices (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL,
    user_id text NOT NULL,
    device_id text NOT NULL,
    hostname text,
    os text,
    arch text,
    cli_version text,
    status text NOT NULL DEFAULT 'connected',
    last_heartbeat_at timestamptz,
    connected_at timestamptz NOT NULL DEFAULT now(),
    disconnected_at timestamptz,
    CONSTRAINT connected_devices_pkey PRIMARY KEY (id),
    CONSTRAINT connected_devices_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id),
    CONSTRAINT connected_devices_account_device_key UNIQUE (account_id, device_id)
);

CREATE INDEX idx_connected_devices_account_status ON public.connected_devices(account_id, status);

CREATE TABLE public.waitlist (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL,
    email text NOT NULL,
    invited_at timestamp,
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT waitlist_pkey PRIMARY KEY (id),
    CONSTRAINT waitlist_email_key UNIQUE (email)
);
