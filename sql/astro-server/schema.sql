-- Astro declarative schema (SDL)
-- This file is the single source of truth for the database schema.
-- Atlas diffs this against the live DB and applies only what changed.

-- Managed workload clusters. astro-server reconciles agent deployments into
-- one of these. `id` is a stable string (e.g. "us-east-1-managed") referenced
-- by `deployments.cluster_id`, `accounts.cluster_id`, and River job payloads.
-- `enabled = false` registers a row that cannot accept new traffic; used to
-- stage a cluster before promoting it.
CREATE TABLE public.clusters (
    id                 varchar(64)  NOT NULL,
    region             varchar(64)  NOT NULL,
    eks_cluster_name   varchar(128) NOT NULL,
    eks_cluster_endpoint varchar    NOT NULL,
    -- EKS API server CA, captured at registration via `aws eks describe-cluster`.
    -- Stored on the row so astro-server's per-cluster client builder doesn't
    -- need cross-account DescribeCluster — astro-server signs an EKS bearer
    -- token with its own IRSA creds and the cluster accepts that principal
    -- via an explicit access entry (see managed-cluster-infra
    -- aws_eks_access_entry.astro_server). NOT NULL with empty default so the
    -- column-add applies safely; clusterstore.validateRequiredFields rejects
    -- empty for new/updated rows.
    eks_cluster_ca     bytea        NOT NULL DEFAULT ''::bytea,
    enabled            boolean      NOT NULL DEFAULT true,
    -- Per-cluster ingress / ALB / cert config. Required for every registered
    -- cluster — clusterstore.Register / Update reject empty values, and the
    -- deployer errors on empty fields at deploy time. The columns default to
    -- '' so the schema diff applies cleanly to existing rows; operators must
    -- backfill values via UpdateCluster before deploys targeting those rows
    -- will succeed. The primary cluster has no row here; it reads env vars
    -- (INGRESS_DOMAIN, ACM_CERTIFICATE_ARN, ...) directly.
    agent_ingress_domain      varchar(253) NOT NULL DEFAULT '',
    agent_acm_certificate_arn varchar      NOT NULL DEFAULT '',
    agent_alb_group_name      varchar(64)  NOT NULL DEFAULT '',
    ingestion_ingress_domain  varchar(253) NOT NULL DEFAULT '',
    ingestion_acm_certificate_arn varchar  NOT NULL DEFAULT '',
    ingestion_alb_group_name      varchar(64) NOT NULL DEFAULT '',
    knowledge_domain          varchar(253) NOT NULL DEFAULT '',
    -- Per-cluster Langfuse PrivateLink + netpol inputs (required for additional
    -- clusters; primary reads LANGFUSE_* / POD_SUBNET_CIDRS env vars).
    langfuse_base_url_ext     varchar(512) NOT NULL DEFAULT '',
    langfuse_vpce_ips         text         NOT NULL DEFAULT '',
    pod_subnet_cidrs          text         NOT NULL DEFAULT '',
    created_at         timestamptz  NOT NULL DEFAULT now(),
    updated_at         timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT clusters_pkey PRIMARY KEY (id)
);

CREATE INDEX idx_clusters_enabled_region ON public.clusters(region) WHERE enabled = true;

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
    cluster_id varchar(64),
    CONSTRAINT accounts_pkey PRIMARY KEY (id),
    CONSTRAINT accounts_name_key UNIQUE (name),
    CONSTRAINT accounts_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES public.clusters(id) ON DELETE RESTRICT
);

CREATE TABLE public.account_profile (
    account_id uuid NOT NULL,
    account_number serial,
    bio varchar(500),
    location varchar(100),
    email varchar(255),
    local_timezone varchar(50),
    pronouns varchar(50),
    website varchar(255),
    social_links text[] NOT NULL DEFAULT '{}',
    blueprint_order text[] NOT NULL DEFAULT '{}',
    CONSTRAINT account_profile_pkey PRIMARY KEY (account_id),
    CONSTRAINT account_profile_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
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
    CONSTRAINT agent_versions_account_id_name_fkey FOREIGN KEY (account_id, name) REFERENCES public.agents(account_id, name) ON UPDATE CASCADE ON DELETE CASCADE
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
    cluster_id varchar(64),
    CONSTRAINT deployments_pkey PRIMARY KEY (id),
    CONSTRAINT deployments_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE,
    CONSTRAINT deployments_source_account_id_fkey FOREIGN KEY (source_account_id) REFERENCES public.accounts(id) ON DELETE SET NULL,
    CONSTRAINT deployments_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES public.clusters(id) ON DELETE RESTRICT
);

CREATE INDEX idx_deployments_account_agent ON public.deployments(account_id, agent_name);

CREATE INDEX idx_deployments_source_account_agent ON public.deployments(source_account_id, agent_name) WHERE source_account_id IS NOT NULL;

CREATE UNIQUE INDEX idx_deployments_live_display_name ON public.deployments(account_id, display_name) WHERE status <> 'undeployed' AND display_name <> '';

CREATE INDEX idx_deployments_cluster_id ON public.deployments(cluster_id) WHERE cluster_id IS NOT NULL;

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

-- Single source of truth for the env each container in a deployment sees.
-- One row per (deployment, role, env name). The K8s ConfigMap and Secret
-- mounted by each container are a pure projection of these rows.
-- See docs/01-spec/unified-deployment-env-spec.md.
CREATE TABLE public.deployment_build_env (
    deployment_id varchar(11) NOT NULL,
    role varchar(64) NOT NULL,
    env_name varchar(255) NOT NULL,
    value_encrypted bytea NOT NULL,
    nonce bytea,
    is_secret boolean NOT NULL,
    source varchar(32) NOT NULL,
    user_var_name varchar(255),
    account_var_ref text,
    optional boolean,
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT deployment_build_env_pkey PRIMARY KEY (deployment_id, role, env_name),
    CONSTRAINT deployment_build_env_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_build_env_user_vars
    ON public.deployment_build_env (deployment_id, user_var_name)
    WHERE source = 'user_var';

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
    CONSTRAINT agent_hearts_agent_fkey FOREIGN KEY (account_id, agent_name) REFERENCES public.agents(account_id, name) ON UPDATE CASCADE ON DELETE CASCADE
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

-- Per-deployment LiteLLM virtual keys. One row per deployment that opts in
-- via agent.ai_gateway: true. Replaces account_ai_gateway — bucketing by
-- deployment lets gateway-side traces and budgets attribute to a specific
-- deployment rather than the whole account. UserID/TeamID on the LiteLLM
-- side remain the account_id so OpenMeter chargeback is invariant; the
-- deployment_id is carried in metadata.tags.
--
-- Lifecycle: minted at first deploy, reused across redeploys (idempotent
-- decrypt-and-return), revoked on undeploy. No rotation today — a future
-- deployment-template API will trigger explicit rotation.
CREATE TABLE public.deployment_ai_gateway (
    deployment_id varchar(11) NOT NULL,
    account_id uuid NOT NULL,
    key_id text NOT NULL,
    encrypted_api_key text NOT NULL,
    encrypted_data_key bytea,
    nonce bytea,
    issued_at timestamp NOT NULL DEFAULT now(),
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT deployment_ai_gateway_pkey PRIMARY KEY (deployment_id),
    CONSTRAINT deployment_ai_gateway_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE,
    CONSTRAINT deployment_ai_gateway_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_ai_gateway_account ON public.deployment_ai_gateway(account_id);

-- Dev keys minted by POST /accounts/:account/ai-gateway-keys for local
-- `astro dev` sessions. Separate from account_ai_gateway (which holds the
-- durable deploy-time key + rotation overlap state) so the dev-key
-- lifecycle stays isolated. Per-(account, user) — each developer gets
-- their own key + audit trail. Reused across `ast dev` invocations
-- while non-expired; replaced when expired with a best-effort upstream
-- revoke of the predecessor.
CREATE TABLE public.account_ai_gateway_dev_keys (
    account_id uuid NOT NULL,
    user_id text NOT NULL,
    key_id text NOT NULL,
    encrypted_api_key text NOT NULL,
    encrypted_data_key bytea,
    nonce bytea,
    expires_at timestamp NOT NULL,
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT account_ai_gateway_dev_keys_pkey PRIMARY KEY (account_id, user_id),
    CONSTRAINT account_ai_gateway_dev_keys_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
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
CREATE INDEX idx_audit_logs_creator_lookup ON public.audit_logs (account_id, action, resource_type, resource_id, created_at ASC);

CREATE TABLE public.github_connections (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL,
    account_name varchar NOT NULL DEFAULT '',
    agent_name varchar NOT NULL,
    workos_user_id text NOT NULL,
    workos_org_id text NOT NULL DEFAULT '',
    repo_full_name varchar NOT NULL,
    branch varchar NOT NULL DEFAULT 'main',
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

CREATE TABLE public.github_webhooks (
    repo_base      varchar     NOT NULL,
    webhook_id     bigint      NOT NULL,
    webhook_secret varchar     NOT NULL,
    created_at     timestamp   NOT NULL DEFAULT now(),
    CONSTRAINT github_webhooks_pkey PRIMARY KEY (repo_base)
);

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
    CONSTRAINT deployment_authorization_grants_subject_check CHECK (subject_type IN ('org', 'user', 'anyone')),
    CONSTRAINT deployment_authorization_grants_adapter_check CHECK (adapter IN ('web', 'slack')),
    -- user grants are now allowed on slack too: the messaging container
    -- forwards (team_id, slack_user_id), the server resolves it to a
    -- WorkOS user via slack_identity_mappings, and the user grant lookup
    -- runs the same path as web. The previous user_web_only_check is gone.
    CONSTRAINT deployment_authorization_grants_anyone_empty_check CHECK (subject_type <> 'anyone' OR subject_id = ''),
    CONSTRAINT deployment_authorization_grants_deployment_fkey FOREIGN KEY (deployment_id)
        REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_authorization_grants_deployment ON public.deployment_authorization_grants(deployment_id);

-- Billing state tables for event-driven compute metering.
-- Tracks what has been billed so inline events and the heartbeat reconciler
-- can calculate fractional CU-hours without double-counting.

CREATE TABLE public.deployment_billing_state (
    deployment_id   varchar(11) NOT NULL,
    component       varchar     NOT NULL,
    billing_active  boolean     NOT NULL DEFAULT false,
    last_emitted_at timestamptz NOT NULL DEFAULT now(),
    stopped_at      timestamptz NULL,
    cpu_request     varchar     NOT NULL DEFAULT '',
    memory_request  varchar     NOT NULL DEFAULT '',
    replicas        int         NOT NULL DEFAULT 1,
    CONSTRAINT deployment_billing_state_pkey PRIMARY KEY (deployment_id, component),
    CONSTRAINT deployment_billing_state_deployment_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_billing_state_active ON public.deployment_billing_state(billing_active) WHERE billing_active = true;

CREATE TABLE public.eval_datasets (
    deployment_id          varchar(11) NOT NULL,
    account_id             uuid        NOT NULL,
    langfuse_dataset_name  varchar     NOT NULL,
    item_count             integer     NOT NULL DEFAULT 0,
    last_trace_at          timestamptz,
    last_sync_attempted_at timestamptz,
    last_synced_at         timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_datasets_pkey PRIMARY KEY (deployment_id),
    CONSTRAINT eval_datasets_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE,
    CONSTRAINT eval_datasets_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

-- No ON DELETE CASCADE: billing rows must outlive the store so the heartbeat
-- can emit the final period after deletion without losing data.
CREATE TABLE public.knowledge_billing_state (
    knowledge_store_id varchar(11) NOT NULL,
    billing_active     boolean     NOT NULL DEFAULT false,
    last_emitted_at    timestamptz NOT NULL DEFAULT now(),
    stopped_at         timestamptz NULL,
    account_id         varchar     NOT NULL DEFAULT '',
    name               varchar     NOT NULL DEFAULT '',
    provider           varchar     NOT NULL,
    CONSTRAINT knowledge_billing_state_pkey PRIMARY KEY (knowledge_store_id)
);

CREATE INDEX idx_knowledge_billing_state_active ON public.knowledge_billing_state(billing_active) WHERE billing_active = true;

-- Maps a Slack user (team_id, slack_user_id) to a WorkOS user_id. Populated
-- when the user connects their Slack account via WorkOS Pipes — the link
-- handler exchanges the Pipes-issued access token for the slack identity via
-- `auth.test`, then upserts a row here.
--
-- Used by the messaging container's authorization callback: a slack request
-- carrying (team_id, slack_user_id) resolves to a WorkOS user_id via this
-- table, which then enriches the candidate set used to match per-user grants.
-- An unmapped slack user falls through to the existing owning-account
-- candidate, so this table is purely additive — no behavior change for users
-- who haven't linked.
--
-- revoked_at is a soft delete: the row is kept for audit / eventual restore
-- when the user disconnects Slack. Lookups filter on revoked_at IS NULL.
-- Linked oauth mappings only. After PR 3 cleanup, observed-anonymous
-- directory entries live in slack_observed_users instead — this table
-- carries the durable WorkOS-user ↔ Slack-identity link captured at
-- link time, and nothing else.
CREATE TABLE public.slack_identity_mappings (
    id                    uuid        NOT NULL DEFAULT gen_random_uuid(),
    team_id               varchar     NOT NULL,
    slack_user_id         varchar     NOT NULL,
    workos_user_id        varchar     NOT NULL,
    organization_id       varchar,
    -- Display fields captured at link time so the settings UI (and audit
    -- logs) can render workspace + handle without re-querying Slack on
    -- every status load. Refreshed on each Upsert.
    team_name             varchar     NOT NULL DEFAULT '',
    team_domain           varchar     NOT NULL DEFAULT '',
    team_icon_url         varchar     NOT NULL DEFAULT '',
    slack_username        varchar     NOT NULL DEFAULT '',
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    revoked_at            timestamptz,
    CONSTRAINT slack_identity_mappings_pkey PRIMARY KEY (id),
    CONSTRAINT slack_identity_mappings_unique UNIQUE (team_id, slack_user_id)
);

CREATE INDEX idx_slack_identity_mappings_workos_user ON public.slack_identity_mappings(workos_user_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_slack_identity_mappings_lookup ON public.slack_identity_mappings(team_id, slack_user_id) WHERE revoked_at IS NULL;

-- slack_observed_users is the directory of Slack identities the server has
-- learned via the /authorize live-ingest path and Slack account-connect
-- users.list sync. Pure derived cache: there is no Astro identity here, only
-- a (team_id, slack_user_id) tuple plus best-effort Slack profile metadata
-- that Insights uses to render and deep-link bare-Slack userIds.
--
-- Split out from slack_identity_mappings because that table was conflating
-- two cardinalities (linked oauth identities + observed-anonymous users)
-- behind a CHECK constraint and source discriminator, blocking safe
-- truncation and forcing two-pass merge logic in the Insights reader. With
-- this table, observed-row truncation is routine; live-ingest refills on
-- every /authorize call.
--
-- Live ingest refreshes activity timestamps; account-connect directory sync
-- refreshes Slack profile fields. Historical backfills can populate the same
-- table without reintroducing observed rows into slack_identity_mappings.
CREATE TABLE public.slack_observed_users (
    team_id              varchar     NOT NULL,
    slack_user_id        varchar     NOT NULL,
    slack_display_name   varchar     NOT NULL DEFAULT '',
    slack_username       varchar     NOT NULL DEFAULT '',
    slack_avatar_url     varchar     NOT NULL DEFAULT '',
    slack_is_bot         boolean     NOT NULL DEFAULT false,
    slack_deleted        boolean     NOT NULL DEFAULT false,
    profile_updated_at   timestamptz,
    first_seen_at        timestamptz NOT NULL DEFAULT now(),
    last_seen_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT slack_observed_users_pkey PRIMARY KEY (team_id, slack_user_id)
);
