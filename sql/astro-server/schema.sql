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
    display_name varchar(64) NOT NULL DEFAULT '',
    avatar_colors jsonb,
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
    archived_at timestamp,
    name_reserved bool NOT NULL DEFAULT false,
    avatar_colors jsonb,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    CONSTRAINT agents_pkey PRIMARY KEY (account_id, name),
    CONSTRAINT agents_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_agents_public ON public.agents(visibility) WHERE visibility = 'public' AND archived_at IS NULL;

CREATE TABLE public.agent_versions (
    account_id uuid NOT NULL,
    name text NOT NULL,
    build_id text NOT NULL,
    ecr_namespace text NOT NULL DEFAULT '',
    spec_json text NOT NULL,
    readme text NOT NULL DEFAULT '',
    agent_card_json jsonb NOT NULL DEFAULT '{}',
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
    source_account_id uuid,
    agent_name varchar NOT NULL,
    build_id varchar NOT NULL,
    namespace varchar NOT NULL,
    display_name varchar(64) NOT NULL DEFAULT '',
    deployment_spec_json text NOT NULL,
    encrypted_data_key bytea,
    kms_key_arn varchar,
    status varchar NOT NULL DEFAULT 'active',
    error_message text,
    error_details jsonb,
    status_changed_at timestamptz NOT NULL DEFAULT now(),
    current_revision int,
    deployed_at timestamp NOT NULL DEFAULT now(),
    undeployed_at timestamp,
    drift_report jsonb,
    drift_checked_at timestamptz,
    avatar_colors jsonb,
    CONSTRAINT deployments_pkey PRIMARY KEY (id),
    CONSTRAINT deployments_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE,
    CONSTRAINT deployments_source_account_id_fkey FOREIGN KEY (source_account_id) REFERENCES public.accounts(id) ON DELETE SET NULL
);

CREATE INDEX idx_deployments_account_agent ON public.deployments(account_id, agent_name);

CREATE INDEX idx_deployments_source_account_agent ON public.deployments(source_account_id, agent_name) WHERE source_account_id IS NOT NULL;

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

CREATE TABLE public.deployment_sidecars (
    id serial NOT NULL,
    deployment_id varchar(11) NOT NULL,
    name varchar NOT NULL,
    component_kind varchar NOT NULL,
    image varchar NOT NULL,
    cpu_request varchar NOT NULL DEFAULT '',
    memory_request varchar NOT NULL DEFAULT '',
    cpu_limit varchar NOT NULL DEFAULT '',
    memory_limit varchar NOT NULL DEFAULT '',
    CONSTRAINT deployment_sidecars_pkey PRIMARY KEY (id),
    CONSTRAINT deployment_sidecars_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_deployment_sidecars_name ON public.deployment_sidecars(deployment_id, name);

CREATE TABLE public.deployment_services (
    id serial NOT NULL,
    workload_id int,
    sidecar_id int,
    name varchar NOT NULL,
    port int NOT NULL,
    target_port int NOT NULL,
    protocol varchar NOT NULL DEFAULT 'http',
    CONSTRAINT deployment_services_pkey PRIMARY KEY (id),
    CONSTRAINT deployment_services_workload_id_fkey FOREIGN KEY (workload_id) REFERENCES public.deployment_workloads(id) ON DELETE CASCADE,
    CONSTRAINT deployment_services_sidecar_id_fkey FOREIGN KEY (sidecar_id) REFERENCES public.deployment_sidecars(id) ON DELETE CASCADE,
    CONSTRAINT deployment_services_owner_check CHECK ((workload_id IS NOT NULL) != (sidecar_id IS NOT NULL))
);

CREATE UNIQUE INDEX idx_deployment_services_workload_name ON public.deployment_services(workload_id, name) WHERE workload_id IS NOT NULL;
CREATE UNIQUE INDEX idx_deployment_services_sidecar_name ON public.deployment_services(sidecar_id, name) WHERE sidecar_id IS NOT NULL;

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

CREATE TABLE public.deployment_variables (
    deployment_id varchar(11) NOT NULL,
    name varchar NOT NULL,
    value text NOT NULL DEFAULT '',
    ref text NOT NULL DEFAULT '',
    secret boolean NOT NULL DEFAULT false,
    optional boolean NOT NULL DEFAULT false,
    targets text[] NOT NULL DEFAULT '{}',
    nonce bytea,
    CONSTRAINT deployment_variables_pkey PRIMARY KEY (deployment_id, name),
    CONSTRAINT deployment_variables_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE TABLE public.deployment_resolved_keys (
    deployment_id varchar(11) NOT NULL,
    configmap_keys text[] NOT NULL DEFAULT '{}',
    secret_keys text[] NOT NULL DEFAULT '{}',
    configmap_hashes jsonb NOT NULL DEFAULT '{}',
    secret_hashes jsonb NOT NULL DEFAULT '{}',
    CONSTRAINT deployment_resolved_keys_pkey PRIMARY KEY (deployment_id),
    CONSTRAINT deployment_resolved_keys_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE TABLE public.deployment_events (
    id bigserial NOT NULL,
    deployment_id varchar(11) NOT NULL,
    status text NOT NULL,
    message text,
    details jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deployment_events_pkey PRIMARY KEY (id),
    CONSTRAINT deployment_events_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_events_deployment ON public.deployment_events(deployment_id);

CREATE TABLE public.deployment_revisions (
    id bigserial NOT NULL,
    deployment_id varchar(11) NOT NULL,
    revision int NOT NULL,
    build_id text NOT NULL,
    spec_json jsonb NOT NULL,
    kms_ciphertext bytea,
    kms_key_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deployment_revisions_pkey PRIMARY KEY (id),
    CONSTRAINT deployment_revisions_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE,
    CONSTRAINT deployment_revisions_unique UNIQUE (deployment_id, revision)
);

CREATE INDEX idx_deployment_revisions_deployment ON public.deployment_revisions(deployment_id);

CREATE TABLE public.scaled_namespaces (
    namespace text NOT NULL,
    deployment_id varchar(11) NOT NULL,
    scaled_down_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT scaled_namespaces_pkey PRIMARY KEY (namespace),
    CONSTRAINT scaled_namespaces_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE TABLE public.namespace_ownership (
    namespace varchar NOT NULL,
    account_id uuid NOT NULL,
    agent_name text NOT NULL,
    deployment_id varchar(11),
    source_account text NOT NULL DEFAULT '',
    scanned_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT namespace_ownership_pkey PRIMARY KEY (namespace),
    CONSTRAINT namespace_ownership_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE,
    CONSTRAINT namespace_ownership_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
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
    CONSTRAINT connected_devices_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE,
    CONSTRAINT connected_devices_account_device_key UNIQUE (account_id, device_id)
);

CREATE INDEX idx_connected_devices_account_status ON public.connected_devices(account_id, status);

CREATE TABLE public.agent_hearts (
    account_id uuid NOT NULL,
    agent_name text NOT NULL,
    user_id text NOT NULL,
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT agent_hearts_pkey PRIMARY KEY (account_id, agent_name, user_id),
    CONSTRAINT agent_hearts_agent_fkey FOREIGN KEY (account_id, agent_name) REFERENCES public.agents(account_id, name) ON DELETE CASCADE
);

CREATE INDEX idx_agent_hearts_agent ON public.agent_hearts(account_id, agent_name);

CREATE TABLE public.quota_increase_requests (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL,
    feature_key varchar NOT NULL,
    current_usage float8 NOT NULL DEFAULT 0,
    current_quota float8,
    requested_amount float8,
    reason text NOT NULL DEFAULT '',
    status varchar NOT NULL DEFAULT 'pending',
    requested_by text NOT NULL,
    resolved_by text,
    resolved_at timestamp,
    resolution_note text,
    grant_amount float8,
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT quota_increase_requests_pkey PRIMARY KEY (id),
    CONSTRAINT quota_increase_requests_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_quota_increase_requests_status ON public.quota_increase_requests(status, created_at);
CREATE INDEX idx_quota_increase_requests_account ON public.quota_increase_requests(account_id);

CREATE TABLE public.agent_message_counts (
    account_id uuid NOT NULL,
    agent_name text NOT NULL,
    lifetime_total bigint NOT NULL DEFAULT 0,
    last_prom_value double precision NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_message_counts_pkey PRIMARY KEY (account_id, agent_name),
    CONSTRAINT agent_message_counts_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE TABLE public.feedback_submissions (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    user_id text NOT NULL,
    user_email text NOT NULL DEFAULT '',
    message text NOT NULL,
    page_url text NOT NULL DEFAULT '',
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT feedback_submissions_pkey PRIMARY KEY (id)
);

CREATE INDEX idx_feedback_submissions_user_created ON public.feedback_submissions(user_id, created_at);

CREATE TABLE public.account_encryption_keys (
    account_id uuid NOT NULL,
    encrypted_data_key bytea NOT NULL,
    kms_key_arn varchar NOT NULL,
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT account_encryption_keys_pkey PRIMARY KEY (account_id),
    CONSTRAINT account_encryption_keys_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE TABLE public.account_variables (
    account_id uuid NOT NULL,
    name varchar NOT NULL,
    value text NOT NULL DEFAULT '',
    secret boolean NOT NULL DEFAULT false,
    nonce bytea,
    description text NOT NULL DEFAULT '',
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT account_variables_pkey PRIMARY KEY (account_id, name),
    CONSTRAINT account_variables_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_account_variables_account ON public.account_variables(account_id);

CREATE TABLE public.account_langfuse (
    account_id uuid NOT NULL,
    langfuse_project_id text NOT NULL,
    langfuse_public_key text NOT NULL,
    langfuse_secret_key text NOT NULL,
    encrypted_data_key bytea,
    nonce bytea,
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT account_langfuse_pkey PRIMARY KEY (account_id),
    CONSTRAINT account_langfuse_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE TABLE public.audit_logs (
    id bigserial NOT NULL,
    account_id uuid NOT NULL,
    actor_id text NOT NULL,
    actor_type text NOT NULL DEFAULT 'user',
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    resource_name text,
    description text,
    metadata jsonb,
    ip_address text,
    user_agent text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT audit_logs_pkey PRIMARY KEY (id)
);

CREATE INDEX idx_audit_logs_account_created ON public.audit_logs (account_id, created_at DESC);
CREATE INDEX idx_audit_logs_account_resource ON public.audit_logs (account_id, resource_type, created_at DESC);
CREATE INDEX idx_audit_logs_actor ON public.audit_logs (actor_id, created_at DESC);
CREATE INDEX idx_audit_logs_created ON public.audit_logs (created_at);

CREATE TABLE public.github_connections (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL,
    account_name varchar NOT NULL DEFAULT '',
    agent_name varchar NOT NULL,
    workos_user_id text NOT NULL,
    workos_org_id text NOT NULL DEFAULT '',
    repo_full_name varchar NOT NULL,
    branch varchar NOT NULL DEFAULT 'main',
    webhook_id bigint,
    webhook_secret varchar NOT NULL,
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT github_connections_pkey PRIMARY KEY (id),
    CONSTRAINT github_connections_account_agent_key UNIQUE (account_id, agent_name),
    CONSTRAINT github_connections_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_github_connections_account ON public.github_connections (account_id);
CREATE INDEX idx_github_connections_repo ON public.github_connections (repo_full_name);
CREATE UNIQUE INDEX idx_github_connections_account_repo ON public.github_connections (account_id, repo_full_name);

CREATE TABLE public.github_builds (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    connection_id uuid NOT NULL,
    account_id uuid NOT NULL,
    agent_name varchar NOT NULL,
    build_id varchar NOT NULL,
    commit_sha varchar NOT NULL,
    branch varchar NOT NULL,
    status varchar NOT NULL DEFAULT 'pending',
    step text NOT NULL DEFAULT '',
    commit_message text NOT NULL DEFAULT '',
    commit_author text NOT NULL DEFAULT '',
    error text,
    enqueued_at timestamp NOT NULL DEFAULT now(),
    completed_at timestamp,
    CONSTRAINT github_builds_pkey PRIMARY KEY (id),
    CONSTRAINT github_builds_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES public.github_connections(id) ON DELETE CASCADE
);

CREATE INDEX idx_github_builds_connection ON public.github_builds (connection_id, enqueued_at DESC);
CREATE INDEX idx_github_builds_account_agent ON public.github_builds (account_id, agent_name, enqueued_at DESC);

CREATE TABLE public.github_build_components (
    id bigserial NOT NULL,
    build_id uuid NOT NULL,
    component_name varchar NOT NULL,
    status varchar NOT NULL DEFAULT 'pending',
    k8s_job_name varchar NOT NULL DEFAULT '',
    logs text NOT NULL DEFAULT '',
    started_at timestamp,
    completed_at timestamp,
    CONSTRAINT github_build_components_pkey PRIMARY KEY (id),
    CONSTRAINT github_build_components_build_fkey FOREIGN KEY (build_id)
        REFERENCES public.github_builds(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_build_components_build_name ON public.github_build_components(build_id, component_name);

CREATE TABLE public.knowledge_stores (
    id                 varchar(11)  NOT NULL,
    account_id         uuid         NOT NULL,
    name               varchar      NOT NULL,
    arn                varchar      NOT NULL,
    provider           varchar      NOT NULL,
    mode               varchar      NOT NULL DEFAULT 'managed',
    status             varchar      NOT NULL DEFAULT 'provisioning',
    storage            varchar      NOT NULL DEFAULT '10Gi',
    storage_class      varchar,
    public             boolean      NOT NULL DEFAULT false,
    public_host        varchar,
    encrypted_data_key bytea,
    kms_key_arn        varchar,
    error              text,
    created_at         timestamptz  NOT NULL DEFAULT now(),
    updated_at         timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT knowledge_stores_pkey PRIMARY KEY (id),
    CONSTRAINT knowledge_stores_arn_key UNIQUE (arn),
    CONSTRAINT knowledge_stores_account_name_key UNIQUE (account_id, name),
    CONSTRAINT knowledge_stores_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_knowledge_stores_account ON public.knowledge_stores(account_id);
CREATE INDEX idx_knowledge_stores_status ON public.knowledge_stores(status);

CREATE TABLE public.knowledge_store_credentials (
    knowledge_store_id varchar(11) NOT NULL,
    key                varchar     NOT NULL,
    value_encrypted    bytea       NOT NULL,
    nonce              bytea       NOT NULL,
    CONSTRAINT knowledge_store_credentials_pkey PRIMARY KEY (knowledge_store_id, key),
    CONSTRAINT knowledge_store_credentials_store_fkey FOREIGN KEY (knowledge_store_id) REFERENCES public.knowledge_stores(id) ON DELETE CASCADE
);

CREATE TABLE public.knowledge_store_endpoints (
    knowledge_store_id varchar(11) NOT NULL,
    cloud_provider     varchar     NOT NULL,
    endpoint_service   varchar     NOT NULL,
    region             varchar     NOT NULL,
    endpoint_id        varchar,
    endpoint_dns       varchar,
    status             varchar     NOT NULL DEFAULT 'connecting',
    error              text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT knowledge_store_endpoints_pkey PRIMARY KEY (knowledge_store_id),
    CONSTRAINT knowledge_store_endpoints_store_fkey FOREIGN KEY (knowledge_store_id) REFERENCES public.knowledge_stores(id) ON DELETE CASCADE
);

CREATE INDEX idx_knowledge_store_endpoints_status ON public.knowledge_store_endpoints(status);

CREATE TABLE public.knowledge_store_bindings (
    deployment_id      varchar(11)  NOT NULL,
    knowledge_name     varchar      NOT NULL,
    knowledge_store_id varchar(11)  NOT NULL,
    created_at         timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT knowledge_store_bindings_pkey PRIMARY KEY (deployment_id, knowledge_name),
    CONSTRAINT knowledge_store_bindings_deployment_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE,
    CONSTRAINT knowledge_store_bindings_store_fkey FOREIGN KEY (knowledge_store_id) REFERENCES public.knowledge_stores(id) ON DELETE RESTRICT
);

CREATE INDEX idx_knowledge_store_bindings_store ON public.knowledge_store_bindings(knowledge_store_id);

-- A row in this table grants the (subject, adapter) pair access to the deployment.
-- A request is allowed iff a matching grant exists. There is no separate policy
-- table — absence of a grant means deny.
--
-- subject_type='account' → subject_id is an accounts.id (uuid as text).
-- subject_type='user'    → subject_id is a workos_user_id.
-- subject_type='anyone'  → subject_id is empty; the row matches any caller.
--
-- `user` grants are restricted to the web adapter — slack identity is opaque,
-- so per-user authz isn't possible there. `anyone` is allowed on either
-- adapter; for slack it collapses to "any caller in the bot's workspace",
-- which is the seeded fresh-deploy default.
--
-- subject_id has no FK because it's polymorphic. Cascade only on deployment_id.
CREATE TABLE public.deployment_authorization_grants (
    id            uuid        NOT NULL DEFAULT gen_random_uuid(),
    deployment_id varchar     NOT NULL,
    subject_type  varchar     NOT NULL,
    subject_id    varchar     NOT NULL,
    adapter       varchar     NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deployment_authorization_grants_pkey PRIMARY KEY (id),
    CONSTRAINT deployment_authorization_grants_unique UNIQUE (deployment_id, subject_type, subject_id, adapter),
    CONSTRAINT deployment_authorization_grants_subject_check CHECK (subject_type IN ('account', 'user', 'anyone')),
    CONSTRAINT deployment_authorization_grants_adapter_check CHECK (adapter IN ('web', 'slack')),
    CONSTRAINT deployment_authorization_grants_user_web_only_check CHECK (subject_type <> 'user' OR adapter = 'web'),
    CONSTRAINT deployment_authorization_grants_anyone_empty_check CHECK (subject_type <> 'anyone' OR subject_id = ''),
    CONSTRAINT deployment_authorization_grants_deployment_fkey FOREIGN KEY (deployment_id)
        REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_authorization_grants_deployment ON public.deployment_authorization_grants(deployment_id);
