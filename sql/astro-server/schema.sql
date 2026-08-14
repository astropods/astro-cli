-- Astro declarative schema (SDL)
-- This file is the single source of truth for the database schema.
-- Atlas diffs this against the live DB and applies only what changed.

-- Managed workload clusters. astro-server reconciles agent deployments into
-- one of these. `id` is a stable string (e.g. "us-east-1-managed") referenced
-- by `deployments.cluster_id`, `accounts.cluster_id`, and River job payloads.
-- Every row present here is usable; a cluster removed from config is deleted
-- by DeleteRemoved (or left in place if still referenced), never disabled.
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
    agent_ingress_domain        varchar(253) NOT NULL DEFAULT '',
    agent_public_ingress_domain varchar(253) NOT NULL DEFAULT '',
    ingestion_ingress_domain    varchar(253) NOT NULL DEFAULT '',
    -- Per-cluster Langfuse PrivateLink + netpol inputs (required for additional
    -- clusters; primary reads LANGFUSE_* / POD_SUBNET_CIDRS env vars).
    langfuse_base_url_ext     varchar(512) NOT NULL DEFAULT '',
    langfuse_vpce_ips         text         NOT NULL DEFAULT '',
    pod_subnet_cidrs          text         NOT NULL DEFAULT '',
    -- IPv6 counterpart to pod_subnet_cidrs, for the tenant NetworkPolicy's
    -- ::/0-except-list. Empty for IPv4-only clusters (every cluster today
    -- except the pm-eu IPv6 pilot).
    pod_subnet_ipv6_cidrs     text         NOT NULL DEFAULT '',
    -- Per-cluster Loki/Prometheus query endpoints. Optional: empty means this
    -- cluster's telemetry is queried through the global LOKI_URL/PROMETHEUS_URL
    -- (the shared/primary observability stack). Set only when a cluster ships
    -- to its own local Loki/VictoriaMetrics behind a dedicated query endpoint.
    loki_url                  varchar(512) NOT NULL DEFAULT '',
    prometheus_url            varchar(512) NOT NULL DEFAULT '',
    -- Private (non-OIDC) address:port for this cluster's tenant-router Envoy,
    -- reached over PrivateLink. Optional: empty means the in-app chat proxy
    -- still relays through the K8s apiserver's services/proxy subresource
    -- instead. See docs/plans/internal-tenant-router-nlb.md (astro-infra repo).
    tenant_router_internal_url varchar(512) NOT NULL DEFAULT '',
    -- Cluster pull credential (CPC), generated at registration. pull_key_hash
    -- is sha256 of its secret portion. See docs/01-spec/registry-pull-through-spec.md.
    pull_credential    text,
    pull_key_hash      bytea,
    -- Set by clusterstore.UpsertFromConfig on every successful boot sync from
    -- astro-infra's cluster config (see
    -- docs/01-spec/cluster-registration-config-spec.md). NULL means this row
    -- was never synced from config — either created before that rollout via
    -- the (now-removed) RegisterCluster RPC, or created directly for tests.
    -- DeleteRemoved only ever touches rows where this is set.
    config_synced_at   timestamptz,
    created_at         timestamptz  NOT NULL DEFAULT now(),
    updated_at         timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT clusters_pkey PRIMARY KEY (id)
);

CREATE TABLE public.accounts (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name varchar(39) NOT NULL,
    type varchar(20) NOT NULL DEFAULT 'personal',
    bifrost_customer_id text,
    -- Hosted billing (Metronome) linkage; populated when BILLING_PROVIDER=metronome.
    metronome_customer_id text,
    -- Stripe customer holding the account's saved payment method. Stripe is used
    -- as a card vault only; Metronome charges this customer's card. Populated on
    -- first payment-method setup when STRIPE_SECRET_KEY is configured.
    stripe_customer_id text,
    -- Stamps when the account was put on the rate card and granted its signup
    -- credit. NULL means the provisioning sweep still owes it work.
    billing_provisioned_at timestamptz,
    deleted_at timestamp,
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    display_name varchar(64) NOT NULL DEFAULT '',
    avatar_colors jsonb,
    -- Stamps when this account's avatar image last changed; drives the `?v=`
    -- cache-busting token appended to its CDN avatar URL. NULL means unknown
    -- (no token emitted); set to now() on the next avatar write.
    avatar_updated_at timestamptz,
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
    local_timezone varchar(50),
    pronouns varchar(50),
    website varchar(255),
    social_links text[] NOT NULL DEFAULT '{}',
    blueprint_order text[] NOT NULL DEFAULT '{}',
    CONSTRAINT account_profile_pkey PRIMARY KEY (account_id),
    CONSTRAINT account_profile_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_accounts_name_prefix ON public.accounts(name text_pattern_ops);

-- The hourly provisioning sweep looks for accounts still owed a contract and
-- credit. Partial so it holds only the pending ones and empties out as they
-- provision, rather than growing with the table.
CREATE INDEX idx_accounts_pending_billing_provision
    ON public.accounts(created_at)
    WHERE billing_provisioned_at IS NULL AND deleted_at IS NULL;

-- Server-owned organization experiments are explicit tenant choices. Missing
-- rows are disabled so new experiments always fail back to current behavior.
CREATE TABLE public.account_experiments (
    account_id uuid NOT NULL,
    experiment text NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_experiments_pkey PRIMARY KEY (account_id, experiment),
    CONSTRAINT account_experiments_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

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

-- Per-account quota overrides. Only overridden (account, resource) pairs get a
-- row; everything else falls back to the system-wide config default. A limit of
-- 0 disables the feature; -1 means unlimited. Admin-editable (via astro-queen).
CREATE TABLE public.account_limits (
    account_id  uuid   NOT NULL,
    resource    text   NOT NULL,
    limit_value bigint NOT NULL,
    CONSTRAINT account_limits_pkey PRIMARY KEY (account_id, resource),
    CONSTRAINT account_limits_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

-- Cached billing/gating status per account (hosted only). Written off the
-- request path by the Metronome/Stripe webhook jobs and the billing.dunning_sweep
-- timer; read by the consumption gate. Absence of a row means 'active'.
-- astro-server never stores or reads a balance — status is driven by webhook
-- signals from Metronome (usage/alerts) and Stripe (payment collection).
CREATE TABLE public.account_billing_status (
    account_id     uuid        NOT NULL,
    status         text        NOT NULL DEFAULT 'active', -- active | past_due | suspended
    reason         text,                                  -- dunning | payment_failed | balance_alert | credits_exhausted | uncollectible
    dunning_since  timestamptz,                           -- set on payment failure, cleared on recovery
    alert_active   boolean     NOT NULL DEFAULT false,    -- last Metronome hard alert, uncleared
    -- Terminal write-off flag: Stripe marked an invoice uncollectible after
    -- exhausting retries. Forces 'suspended' immediately (bypasses the dunning
    -- grace); cleared only on recovery or when the invoice is voided.
    force_suspended boolean    NOT NULL DEFAULT false,
    -- Signup credit is spent. Gates only while has_payment_method is false;
    -- adding a card is what turns the account into pay-as-you-go. Metronome
    -- fires no recovery event, so this clears on our own credit grant.
    credits_exhausted  boolean NOT NULL DEFAULT false,
    -- A card is vaulted in Stripe and linked to the billing provider.
    has_payment_method boolean NOT NULL DEFAULT false,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_billing_status_pkey PRIMARY KEY (account_id),
    CONSTRAINT account_billing_status_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE TABLE public.account_member_workos (
    account_id uuid NOT NULL,
    user_id text NOT NULL,
    workos_membership_id text NOT NULL,
    CONSTRAINT account_member_workos_pkey PRIMARY KEY (account_id, user_id),
    CONSTRAINT account_member_workos_fkey FOREIGN KEY (account_id, user_id) REFERENCES public.account_members(account_id, user_id) ON DELETE CASCADE,
    CONSTRAINT account_member_workos_workos_membership_id_key UNIQUE (workos_membership_id)
);

-- Local mirror of member (WorkOS user) email addresses. This is the join key
-- for attributing external dev-tool telemetry (stamped with user.email) to a
-- member without a per-request WorkOS lookup. One-to-many: a user may have
-- several emails; `source` distinguishes WorkOS-synced emails from ones added
-- directly (direct-add is not yet implemented). Kept fresh by auth-time capture
-- (login + account create) and a periodic reconcile (internal/riverqueue).
CREATE TABLE public.account_member_emails (
    id         uuid        NOT NULL DEFAULT gen_random_uuid(),
    user_id    text        NOT NULL,
    email      text        NOT NULL,
    source     text        NOT NULL DEFAULT 'workos',
    verified   boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_member_emails_pkey PRIMARY KEY (id),
    CONSTRAINT account_member_emails_email_key UNIQUE (email)
);

CREATE INDEX idx_account_member_emails_user ON public.account_member_emails(user_id);

-- Notification preferences are owned by Novu (per-workflow defaults + per-
-- subscriber overrides), not in this schema. See docs/01-spec/notifications-spec.md.

-- Observation alert firing state. One row per (deployment_id, workload,
-- condition) while a resource/health condition is breaching for that workload:
-- active_since drives the `for` sustained window; notified marks that the firing
-- edge was handled. The evaluator resolves breaching pods to their workload
-- (the app.kubernetes.io/component label, e.g. "agent", "model-x") so the UI can
-- attribute an alert, while still emitting one notification per (deployment,
-- condition) episode. The row is deleted when the condition resolves for that
-- workload. See internal/observation.
CREATE TABLE public.deployment_alert_state (
    deployment_id text        NOT NULL,
    workload      text        NOT NULL,
    condition     text        NOT NULL,
    active_since  timestamptz NOT NULL,
    notified      boolean     NOT NULL DEFAULT false,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deployment_alert_state_pkey PRIMARY KEY (deployment_id, workload, condition)
);

-- Per-(deployment, condition) notification control for observation alerts. One
-- row carries both the daily-cap ledger and any admin mute; both columns are
-- nullable and independent. Unlike deployment_alert_state (deleted on resolve),
-- this row survives resolves so its suppression state persists across episodes.
--   last_notified_at: when the condition last notified (NULL = never). Caps a
--     flapping deployment to one alert per (deployment, condition) per rolling
--     24h — the evaluator only sends when this is NULL or older than the window.
--   muted_until: while in the future, an admin has silenced this condition. The
--     evaluator still detects and tracks the breach (the deployment_alert_state
--     row stays pending), but suppresses the send, so the condition re-fires
--     automatically once the mute expires. Scoped per (deployment, condition) to
--     match the dedup scope — all workloads of a condition mute together.
-- See internal/observation.
CREATE TABLE public.deployment_alert_notifications (
    deployment_id    text        NOT NULL,
    condition        text        NOT NULL,
    last_notified_at timestamptz,
    muted_until      timestamptz,
    CONSTRAINT deployment_alert_notifications_pkey PRIMARY KEY (deployment_id, condition)
);

-- Per-(deployment, evaluator) configuration-drift check state. An evaluator
-- (internal/deployeval) is a named, declarative check-and-fix pair for one
-- kind of drift (e.g. "does this deployment's Ingress reflect the current
-- tenant-router routing shape"); an operator runs it on demand from the
-- astro-queen Deployments page and fixes drifted deployments individually.
-- status is one of 'ok' | 'drifted' | 'fix_failed'. fixed_at is set whenever
-- a Fix was attempted, whether or not it fully resolved the drift.
CREATE TABLE public.deployment_evaluator_state (
    deployment_id text        NOT NULL,
    evaluator_id  text        NOT NULL,
    status        text        NOT NULL,
    detail        text        NOT NULL DEFAULT '',
    checked_at    timestamptz NOT NULL DEFAULT now(),
    fixed_at      timestamptz,
    CONSTRAINT deployment_evaluator_state_pkey PRIMARY KEY (deployment_id, evaluator_id)
);

CREATE INDEX idx_deployment_evaluator_state_evaluator_status
    ON public.deployment_evaluator_state(evaluator_id, status);

-- At most one WorkOS-synced email per user. Other sources (future direct-add)
-- are intentionally not covered, so a user may still hold several such emails.
CREATE UNIQUE INDEX account_member_emails_user_workos_key ON public.account_member_emails(user_id) WHERE source = 'workos';

-- Records reconcile attempts for members whose email couldn't be resolved from
-- WorkOS, so the backfill job backs off instead of re-querying them every run.
CREATE TABLE public.member_email_reconcile_attempts (
    user_id      text        NOT NULL,
    attempted_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT member_email_reconcile_attempts_pkey PRIMARY KEY (user_id)
);

CREATE TABLE public.agents (
    account_id uuid NOT NULL,
    name text NOT NULL,
    registry text NOT NULL,
    visibility varchar(10) NOT NULL DEFAULT 'private',
    archived_at timestamp,
    name_reserved bool NOT NULL DEFAULT false,
    avatar_colors jsonb,
    -- Stamps when this agent's avatar image last changed; drives the `?v=`
    -- cache-busting token appended to its CDN avatar URL. NULL means unknown
    -- (no token emitted); set to now() on the next avatar write.
    avatar_updated_at timestamptz,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    CONSTRAINT agents_pkey PRIMARY KEY (account_id, name),
    CONSTRAINT agents_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_agents_public ON public.agents(visibility) WHERE visibility = 'public' AND archived_at IS NULL;
CREATE INDEX idx_agents_active_name_cursor
    ON public.agents(name, account_id)
    WHERE archived_at IS NULL;

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

CREATE INDEX idx_versions_agent
    ON public.agent_versions(account_id, name, published_at DESC)
    INCLUDE (build_id);

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
    -- Stamps when this deployment's avatar image last changed; drives the `?v=`
    -- cache-busting token appended to its CDN avatar URL. NULL means unknown
    -- (no token emitted); set to now() on the next avatar write.
    avatar_updated_at timestamptz,
    cluster_id varchar(64),
    -- WorkOS user ID that created the deployment. Resource-role reconciliation
    -- resolves this user to the account's current organization membership.
    deployed_by text,
    CONSTRAINT deployments_pkey PRIMARY KEY (id),
    CONSTRAINT deployments_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE,
    CONSTRAINT deployments_source_account_id_fkey FOREIGN KEY (source_account_id) REFERENCES public.accounts(id) ON DELETE SET NULL,
    CONSTRAINT deployments_cluster_id_fkey FOREIGN KEY (cluster_id) REFERENCES public.clusters(id) ON DELETE RESTRICT
);

CREATE INDEX idx_deployments_account_agent ON public.deployments(account_id, agent_name);

-- Supports membership-scoped global keyset pages without walking each account
-- independently. The unique id is the stable deployed_at tiebreaker.
CREATE INDEX idx_deployments_visible_account_cursor
    ON public.deployments(account_id, deployed_at DESC, id DESC)
    WHERE status <> 'undeployed';

-- Broad membership scopes can read directly in global cursor order, while the
-- included account ID keeps membership filtering available to the index scan.
CREATE INDEX idx_deployments_visible_global_cursor
    ON public.deployments(deployed_at DESC, id DESC)
    INCLUDE (account_id)
    WHERE status <> 'undeployed';

CREATE INDEX idx_deployments_source_account_agent ON public.deployments(source_account_id, agent_name) WHERE source_account_id IS NOT NULL;

CREATE UNIQUE INDEX idx_deployments_live_display_name ON public.deployments(account_id, display_name) WHERE status <> 'undeployed' AND display_name <> '';

CREATE INDEX idx_deployments_cluster_id ON public.deployments(cluster_id) WHERE cluster_id IS NOT NULL;

-- Transactional desired state for the deployment resource in WorkOS FGA. The
-- deployment transaction is authoritative; a River worker applies this state
-- after commit and records failures here for retry without rolling Astro back.
CREATE TABLE public.deployment_fga_sync (
    deployment_id varchar(11) NOT NULL,
    desired_state text NOT NULL,
    desired_version bigint NOT NULL DEFAULT 1,
    synced_state text,
    synced_version bigint,
    -- Resource lifecycle can converge while a current member's creator role
    -- remains eligible for low-frequency retry after membership-mirror lag.
    creator_assignment_pending boolean NOT NULL DEFAULT false,
    attempt_count int NOT NULL DEFAULT 0,
    last_error text,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    synced_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deployment_fga_sync_pkey PRIMARY KEY (deployment_id),
    CONSTRAINT deployment_fga_sync_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE,
    CONSTRAINT deployment_fga_sync_desired_state_check CHECK (desired_state IN ('registered', 'deleted')),
    CONSTRAINT deployment_fga_sync_synced_state_check CHECK (synced_state IS NULL OR synced_state IN ('registered', 'deleted'))
);

CREATE INDEX idx_deployment_fga_sync_pending
    ON public.deployment_fga_sync(next_attempt_at, updated_at)
    WHERE synced_state IS DISTINCT FROM desired_state
       OR synced_version IS DISTINCT FROM desired_version
       OR creator_assignment_pending;

-- Who gets alerted about a deployment. A member becomes a watcher implicitly by
-- acting on it (deploying, changing config, rolling back, …); registration is
-- driven off the audit-log seam, so any action recorded there enrolls its actor.
--   muted: the member's sticky opt-out. Registration never clears it, so a later
--     deploy does not silently resubscribe someone who unwatched. This is why an
--     opt-out keeps the row instead of deleting it.
--   reason: the audit action that first enrolled them, for explaining "why am I
--     getting this?" — not used in routing.
-- Alerts resolve to unmuted watchers; a deployment with none falls back to the
-- account's managers. See internal/watcher and internal/notify.
CREATE TABLE public.deployment_watchers (
    deployment_id  varchar(11) NOT NULL,
    user_id        text        NOT NULL,
    account_id     uuid        NOT NULL,
    reason         text        NOT NULL DEFAULT '',
    muted          boolean     NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_active_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deployment_watchers_pkey PRIMARY KEY (deployment_id, user_id),
    CONSTRAINT deployment_watchers_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE,
    CONSTRAINT deployment_watchers_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

-- Recipient resolution reads unmuted watchers for one deployment on every alert.
CREATE INDEX idx_deployment_watchers_active
    ON public.deployment_watchers(deployment_id)
    WHERE NOT muted;

-- "What am I watching?" for the per-user API.
CREATE INDEX idx_deployment_watchers_user ON public.deployment_watchers(user_id, created_at DESC);

-- Normalized deployment spec tables (Phase 1: dual-write alongside deployment_spec_json)

CREATE TABLE public.deployment_workloads (
    id serial NOT NULL,
    deployment_id varchar(11) NOT NULL,
    name varchar NOT NULL,
    component_kind varchar NOT NULL,
    component_key varchar NOT NULL DEFAULT '',
    provider varchar,
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

-- deployment_workload_status is the persisted, observed health of each workload
-- in a deployment, written by the event-driven deployment controller (astro-worker)
-- from live K8s state. It is a materialized view of cluster reality — the API
-- reads it instead of querying K8s per request, and it is the aggregation source
-- for deployment-level status. One row per (deployment, workload).
CREATE TABLE public.deployment_workload_status (
    deployment_id varchar(11) NOT NULL,
    workload_name varchar NOT NULL,
    workload_type varchar NOT NULL,
    phase varchar NOT NULL,
    reason varchar NOT NULL DEFAULT '',
    message text NOT NULL DEFAULT '',
    observed_ready int NOT NULL DEFAULT 0,
    observed_desired int NOT NULL DEFAULT 0,
    observed_generation bigint NOT NULL DEFAULT 0,
    observed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deployment_workload_status_pkey PRIMARY KEY (deployment_id, workload_name),
    CONSTRAINT deployment_workload_status_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_workload_status_deployment ON public.deployment_workload_status(deployment_id);

-- deployment_runtime_status is the persisted, live K8s runtime snapshot of a
-- deployment, written by the event-driven deployment controller (astro-worker)
-- from its informer caches. It is the read model behind GET /deployments/:id/
-- runtime: the API deserializes this document instead of hitting the K8s API
-- per request. Stored as one JSONB document per deployment (read whole, rendered
-- whole — never queried by inner field), so more pods, containers, or services
-- are just more JSON, never a schema change. One row per deployment.
CREATE TABLE public.deployment_runtime_status (
    deployment_id varchar(11)  NOT NULL,
    snapshot      jsonb        NOT NULL,
    observed_at   timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT deployment_runtime_status_pkey PRIMARY KEY (deployment_id),
    CONSTRAINT deployment_runtime_status_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
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

-- Account-scoped OTel ingest keys. Set as the forced telemetry credential on
-- developer machines (e.g. Claude Code via Anthropic managed settings); the
-- ingest endpoint hashes the presented bearer key and looks it up here to
-- resolve the account. Ingest-only by construction — grants no read access, so
-- there is no scope/permission column. token_hash is sha256(plaintext): the
-- ingest path verifies per batch and needs an indexed, cache-friendly lookup,
-- which a per-hash-salted bcrypt could not provide. Plaintext is shown once at
-- creation and never stored.
CREATE TABLE public.otel_ingest_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id   uuid NOT NULL REFERENCES public.accounts(id) ON DELETE CASCADE,
    name         text NOT NULL,
    token_hash   bytea NOT NULL,
    token_prefix text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   text,                                 -- WorkOS user id of the creator
    last_used_at timestamptz,
    revoked_at   timestamptz,
    -- Per-key privacy exclusions: lowercased user emails whose full-text
    -- content (prompts, responses, tool calls) is dropped at ingest. Usage
    -- metadata still flows; excluded users never reach the trace explorer.
    excluded_emails text[] NOT NULL DEFAULT '{}'
);
CREATE UNIQUE INDEX otel_ingest_tokens_token_hash_idx ON public.otel_ingest_tokens (token_hash);
CREATE INDEX otel_ingest_tokens_account_idx ON public.otel_ingest_tokens (account_id);

-- Per-deployment LiteLLM virtual keys. One row per deployment that opts in
-- via agent.ai_gateway: true. Replaces account_ai_gateway — bucketing by
-- deployment lets gateway-side traces and budgets attribute to a specific
-- deployment rather than the whole account. UserID/TeamID on the LiteLLM
-- side remain the account_id so per-account chargeback is invariant; the
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

-- Long-lived account-scoped virtual key used by Astro's internal eval-dataset
-- judge. It shares the account's Bifrost customer and budget but remains
-- lifecycle-isolated from deployment and developer keys.
CREATE TABLE public.account_llm_judge_keys (
    account_id uuid NOT NULL,
    key_id text NOT NULL,
    encrypted_api_key text NOT NULL,
    encrypted_data_key bytea,
    nonce bytea,
    issued_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_llm_judge_keys_pkey PRIMARY KEY (account_id),
    CONSTRAINT account_llm_judge_keys_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
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
CREATE INDEX idx_audit_logs_resource_latest
    ON public.audit_logs (account_id, resource_type, resource_id, created_at DESC, id DESC)
    INCLUDE (actor_id);

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
    -- Provider-agnostic origin annotations, e.g. Supabase import details:
    -- { "source": "supabase", "supabase_project_id": "...", "region": "...",
    --   "organization_id": "..." }. A Supabase store is a plain Postgres store
    -- (provider='postgres'); this is the only record of its Supabase origin.
    annotations        jsonb,
    created_at         timestamptz  NOT NULL DEFAULT now(),
    updated_at         timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT knowledge_stores_pkey PRIMARY KEY (id),
    CONSTRAINT knowledge_stores_arn_key UNIQUE (arn),
    CONSTRAINT knowledge_stores_account_name_key UNIQUE (account_id, name),
    CONSTRAINT knowledge_stores_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE INDEX idx_knowledge_stores_account_cursor
    ON public.knowledge_stores(account_id, created_at DESC, id DESC);
CREATE INDEX idx_knowledge_stores_global_cursor
    ON public.knowledge_stores(created_at DESC, id DESC)
    INCLUDE (account_id);
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
    CONSTRAINT deployment_authorization_grants_adapter_check CHECK (adapter IN ('web', 'slack', 'custom')),
    -- user grants are now allowed on slack too: the messaging container
    -- forwards (team_id, slack_user_id), the server resolves it to a
    -- WorkOS user via slack_identity_mappings, and the user grant lookup
    -- runs the same path as web. The previous user_web_only_check is gone.
    CONSTRAINT deployment_authorization_grants_anyone_empty_check CHECK (subject_type <> 'anyone' OR subject_id = ''),
    CONSTRAINT deployment_authorization_grants_deployment_fkey FOREIGN KEY (deployment_id)
        REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_authorization_grants_deployment ON public.deployment_authorization_grants(deployment_id);

-- NOTE: Deployment chat is no longer persisted in astro-server's database.
-- Chat conversation metadata and message bodies live in the messaging sidecar's
-- deployment-local SQLite store on the agent's durable persistent disk (PVC),
-- which survives pod reschedules. The sidecar has no Langfuse access — durability
-- comes from the disk, not trace restore. No chat content is stored in RDS (GDPR
-- posture).

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
    id                     uuid        NOT NULL DEFAULT gen_random_uuid(),
    deployment_id          varchar(11) NOT NULL,
    account_id             uuid        NOT NULL,
    langfuse_dataset_name  varchar     NOT NULL,
    good_count             integer     NOT NULL DEFAULT 0,
    bad_count              integer     NOT NULL DEFAULT 0,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_datasets_pkey PRIMARY KEY (id),
    CONSTRAINT eval_datasets_deployment_id_key UNIQUE (deployment_id),
    CONSTRAINT eval_datasets_good_count_check CHECK (good_count >= 0),
    CONSTRAINT eval_datasets_bad_count_check CHECK (bad_count >= 0),
    CONSTRAINT eval_datasets_deployment_id_fkey FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE,
    CONSTRAINT eval_datasets_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);

CREATE TABLE public.eval_dataset_judgments (
    eval_dataset_id uuid        NOT NULL,
    trace_id        text        NOT NULL,
    verdict         text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_dataset_judgments_pkey PRIMARY KEY (eval_dataset_id, trace_id),
    CONSTRAINT eval_dataset_judgments_eval_dataset_id_fkey FOREIGN KEY (eval_dataset_id) REFERENCES public.eval_datasets(id) ON DELETE CASCADE,
    CONSTRAINT eval_dataset_judgments_verdict_check CHECK (verdict IN ('good', 'bad', 'unknown'))
);

CREATE TABLE public.eval_dataset_judgment_reasons (
    eval_dataset_id uuid        NOT NULL,
    trace_id        text        NOT NULL,
    dimension_key   text        NOT NULL,
    dimension_value numeric     NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_dataset_judgment_reasons_pkey PRIMARY KEY (eval_dataset_id, trace_id, dimension_key),
    CONSTRAINT eval_dataset_judgment_reasons_judgment_fkey FOREIGN KEY (eval_dataset_id, trace_id) REFERENCES public.eval_dataset_judgments(eval_dataset_id, trace_id) ON DELETE CASCADE,
    CONSTRAINT eval_dataset_judgment_reasons_value_check CHECK (dimension_value BETWEEN -1 AND 1)
);

CREATE TABLE public.eval_dataset_judgment_predictions (
    eval_dataset_id uuid        NOT NULL,
    trace_id        text        NOT NULL,
    trace_timestamp timestamptz NOT NULL,
    verdict_score   numeric     NOT NULL,
    confidence      integer     NOT NULL,
    explanation     text        NOT NULL DEFAULT '',
    judge_version   text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_dataset_judgment_predictions_pkey PRIMARY KEY (eval_dataset_id, trace_id),
    CONSTRAINT eval_dataset_judgment_predictions_dataset_fkey FOREIGN KEY (eval_dataset_id) REFERENCES public.eval_datasets(id) ON DELETE CASCADE,
    CONSTRAINT eval_dataset_judgment_predictions_score_check CHECK (verdict_score BETWEEN -1 AND 1),
    CONSTRAINT eval_dataset_judgment_predictions_confidence_check CHECK (confidence BETWEEN 0 AND 100),
    CONSTRAINT eval_dataset_judgment_predictions_explanation_check CHECK (char_length(explanation) <= 240)
);

CREATE INDEX eval_dataset_judgment_predictions_trace_timestamp_idx
    ON public.eval_dataset_judgment_predictions
    (eval_dataset_id, trace_timestamp DESC, trace_id DESC);

CREATE TABLE public.eval_dataset_judgment_prediction_criteria (
    eval_dataset_id uuid        NOT NULL,
    trace_id        text        NOT NULL,
    dimension_key   text        NOT NULL,
    dimension_value numeric     NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_dataset_judgment_prediction_criteria_pkey PRIMARY KEY (eval_dataset_id, trace_id, dimension_key),
    CONSTRAINT eval_dataset_judgment_prediction_criteria_prediction_fkey FOREIGN KEY (eval_dataset_id, trace_id) REFERENCES public.eval_dataset_judgment_predictions(eval_dataset_id, trace_id) ON DELETE CASCADE,
    CONSTRAINT eval_dataset_judgment_prediction_criteria_value_check CHECK (dimension_value BETWEEN -1 AND 1)
);

CREATE TABLE public.eval_dataset_prediction_requests (
    eval_dataset_id uuid        NOT NULL,
    trace_id        text        NOT NULL,
    status          text        NOT NULL DEFAULT 'queued',
    error_message   text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT eval_dataset_prediction_requests_pkey PRIMARY KEY (eval_dataset_id, trace_id),
    CONSTRAINT eval_dataset_prediction_requests_dataset_fkey FOREIGN KEY (eval_dataset_id) REFERENCES public.eval_datasets(id) ON DELETE CASCADE,
    CONSTRAINT eval_dataset_prediction_requests_status_check CHECK (status IN ('queued', 'in_progress', 'completed', 'failed'))
);

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

-- insights_usage_daily is the durable daily-grain fact table behind the
-- account Insights page. It replaces re-aggregating 90 days of Langfuse data
-- from scratch every 6h and caching the rendered page in Redis: a day is
-- rolled up once and never recomputed, so serving is a SQL aggregate and
-- freshness stops being a function of recompute cost. Design doc:
-- docs/01-spec/insights-rollup-spec.md.
--
-- `grain` discriminates two descriptions of the SAME spend, so summing across
-- grains double-counts:
--   'usage' — (deployment_id, actor_kind, actor_key). The measure grain; every
--             surface on the page is a GROUP BY over it. Sourced from one
--             Langfuse traces-view query grouped by [tags, userId], which is
--             safe to sum because `tags` is not an explodeArray dimension and
--             so groups by the whole tag array rather than fanning out per tag.
--   'model'  — (model). Exists only for the Models view. Necessarily read from
--             the observations view, whose cost does not reconcile with the
--             traces view, which is why it is never mixed with 'usage'.
-- Every query against this table must filter on `grain`; the store's query
-- builders take it as a required argument so there is no default to forget.
--
-- Sentinels are '' rather than NULL because these are primary-key columns, and
-- some of the absences are meaningful: deployment_id = '' is a trace with no
-- deployment tag (or a dev-tool source, which has no deployment), and
-- actor_kind = 'system' with an empty actor_key is a trace with no user —
-- exactly the pinned system row the page renders today.
--
-- No FK on deployment_id, deliberately: usage history must outlive the
-- deployment it describes, so deleted agents keep their spend. The read path
-- LEFT JOINs deployments and treats a missing row as deleted.
CREATE TABLE public.insights_usage_daily (
    account_id      uuid          NOT NULL,
    -- grain precedes day in the PK so the index prefix matches the serving
    -- predicate: two equality columns, then a range scan on day.
    grain           varchar(16)   NOT NULL,
    day             date          NOT NULL,
    source          varchar(64)   NOT NULL,           -- 'agents' | 'claude-code' | …
    -- Wider than deployments.id (varchar(11)) on purpose: this value is parsed
    -- out of an upstream tag string, and a malformed tag must not abort the
    -- whole day's transaction with a value-too-long error.
    deployment_id   varchar(64)   NOT NULL DEFAULT '',
    actor_kind      varchar(16)   NOT NULL DEFAULT '',
    -- Full stable identity key, not a raw user id: 'member:<user_id>',
    -- 'slack:<team_id>:<user_id>'. Mirrors insightIdentityRowKey, which is what
    -- lets dev-tool spend merge into the same member's row as agent spend. The
    -- Slack team is part of the key because one Slack user id in two workspaces
    -- is two different people.
    actor_key       varchar(256)  NOT NULL DEFAULT '',
    model           varchar(128)  NOT NULL DEFAULT '',

    -- requests is permanently 0 for dev-tool sources: no such metric is
    -- emitted. That is real data, not a pending value, so per-request derived
    -- columns must guard the denominator.
    requests        bigint        NOT NULL DEFAULT 0,
    input_tokens    bigint        NOT NULL DEFAULT 0,
    output_tokens   bigint        NOT NULL DEFAULT 0,
    total_tokens    bigint        NOT NULL DEFAULT 0,
    -- numeric, not float: summing millions of float rows accumulates drift and
    -- these are money-shaped values.
    cost_usd        numeric(18,6) NOT NULL DEFAULT 0,
    -- Forward-declared for the future ingest-side producer and left empty by
    -- the ETL. Langfuse does expose a `histogram` aggregation, but it compiles
    -- to ClickHouse's adaptive histogram(bins)(x), whose bin boundaries are
    -- derived from each query's own data — so stored bins cannot be merged
    -- across days, which is the same defect as storing a scalar p95. Until an
    -- ingest producer can emit fixed boundaries, p95 stays on the existing
    -- whole-period Langfuse query.
    latency_buckets bigint[]      NOT NULL DEFAULT '{}',
    last_seen_at    timestamptz,

    computed_at     timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT insights_usage_daily_pkey
        PRIMARY KEY (account_id, grain, day, source, deployment_id, actor_kind, actor_key, model),
    CONSTRAINT insights_usage_daily_account_id_fkey
        FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE,
    CONSTRAINT insights_usage_daily_grain_check
        CHECK (grain IN ('usage', 'model')),
    CONSTRAINT insights_usage_daily_actor_kind_check
        CHECK (actor_kind IN ('', 'member', 'slack', 'system', 'unidentified')),
    -- Enforce which dimensions each grain may populate, so a producer bug fails
    -- at insert rather than silently double-counting at read time.
    CONSTRAINT insights_usage_daily_shape_check CHECK (
        (grain = 'usage' AND model = '')
        OR (grain = 'model' AND model <> '' AND deployment_id = ''
            AND actor_kind = '' AND actor_key = '')
    )
);

-- insights_rollup_state is the per-(account, source) watermark driving the
-- incremental roll-up. rolled_up_through is the last day considered complete;
-- each tick re-rolls from there minus a small trailing window, because traces
-- arrive late (agents buffer, collectors retry, laptops go offline).
--
-- Scoped to (account_id, source) and not grain: one producer run emits every
-- grain for its source, so a per-grain watermark could only ever disagree with
-- itself. A stalled watermark is a first-class visible state — it surfaces to
-- the page as `as_of` rather than silently serving a stale cache entry.
CREATE TABLE public.insights_rollup_state (
    account_id         uuid        NOT NULL,
    source             varchar(64) NOT NULL,
    rolled_up_through  date,
    last_run_at        timestamptz,
    last_error         text        NOT NULL DEFAULT '',
    consecutive_errors int         NOT NULL DEFAULT 0,
    CONSTRAINT insights_rollup_state_pkey PRIMARY KEY (account_id, source),
    CONSTRAINT insights_rollup_state_account_id_fkey
        FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);
