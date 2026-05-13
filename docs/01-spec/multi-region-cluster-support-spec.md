# Multi-Region Cluster Support — Astro-Server Contract

**Version**: 2.0
**Status**: Draft
**Date**: 2026-05-13
**Plan**: [m0_phase_1_pr_sequence_60a44a07.plan.md](../../../../.cursor/plans/m0_phase_1_pr_sequence_60a44a07.plan.md)

## Overview

`astro-server` today reconciles workloads into exactly one managed Kubernetes cluster. The cluster identity is a global singleton — chosen at boot from `EKS_CLUSTER_NAME` / `K8S_MASTER_URL` env vars and held as a single `k8s.ClusterClient` shared by every handler and River worker.

This spec defines the astro-server-side contract for running against **a primary cluster plus zero or more additional clusters**. The primary stays env-var-defined (the cluster astro-server itself is deployed into / against). Additional clusters are registered at runtime via the admin API and persisted as rows in `public.clusters`. Deployments either route to the primary (`cluster_id IS NULL`) or pin to a specific additional cluster (`cluster_id` matches a row).

This is **Milestone 0, Phase 1**. The application contract changes only — no new infrastructure is provisioned, and the default single-cluster topology continues to run unmodified in production. Subsequent phases provision additional clusters and validate cross-region behavior on top of this contract.

## Terminology

- **Primary cluster** — the cluster astro-server is configured against via env vars (`EKS_CLUSTER_NAME` / `K8S_MASTER_URL` / `AWS_REGION` in EKS mode, kubeconfig in local mode). There is exactly one per astro-server process. It is **not** a row in the `clusters` table; it is defined by the deployment artifact.
- **Additional cluster** — a runtime-registered cluster recorded as a row in `public.clusters`. Added by operators via the admin API; reachable from astro-server using the row's coordinates.
- **Cluster row** — a row in `public.clusters` describing one *additional* cluster: identity, region, EKS coordinates, enabled flag.
- **ClusterClient** — existing Go interface in `internal/k8s` that abstracts Kubernetes connectivity (`Clientset`, `Config`, `CheckHealth`, `GetServerVersion`, `DiagnoseConnection`).
- **Registry** — the in-process owner of all live `ClusterClient`s. Always holds the primary; future PRs add a cache of additional-cluster clients keyed by `cluster_id`.
- **Disabled cluster** — an additional cluster row with `enabled = false`. Visible to operators, but the registry will not hand out a client for it and the deploy path rejects requests targeting it. The primary cannot be "disabled" via the API — disabling the primary means redeploying astro-server with new env vars.

## Goals

1. **Primary stays a deployment artifact.** The cluster astro-server is built against lives in env vars / kubeconfig, not in any application-managed table. Changing the primary is a redeploy.
2. **Additional clusters are runtime data.** Operators add, enable, disable, deregister them through the admin API while astro-server runs. They are rows in `public.clusters`.
3. **One owner of K8s clients at runtime.** The `k8s.Registry` is the only place that constructs `ClusterClient`s. Handlers and workers consume the registry; they never call `NewClusterClient` themselves.
4. **Single-cluster behavior unchanged.** Production with exactly one cluster behaves identically before and after this work. Every handler/worker receives `registry.Default()`, which is the primary's `ClusterClient` — equivalent to the legacy singleton.
5. **Per-deployment routing on the way.** `deployments.cluster_id` is the seam: `NULL` means primary; a non-null id pins the deployment to a specific additional cluster. Worker payloads and handler resolution will grow to honor it in subsequent PRs.

## Non-Goals

1. **Cross-account / cross-VPC connectivity.** Every cluster is assumed reachable using astro-server's existing IAM identity (same AWS account, same auth path as today). The `sts:AssumeRole` story for customer-managed clusters is a later milestone.
2. **Re-routing existing deployments.** Deployments that exist at cutover have `cluster_id = NULL` and continue to be served by the primary. There is no bulk migration of `cluster_id` values; re-targeting a deployment to an additional cluster requires a redeploy.
3. **Cluster auto-discovery.** Registration of additional clusters is an explicit operator action via admin gRPC / astro-queen. The server does not enumerate EKS clusters in the AWS account.
4. **Web UI for cluster selection on the deploy form.** The deploy payload accepts an optional `cluster_id` for API callers; the astro-client UX for choosing it is a separate frontend effort.
5. **Infrastructure provisioning.** This spec covers only the astro-server application contract. Terraform module changes, IAM/IRSA generalisation, and bringing up additional clusters are handled by separate work that consumes this contract.
6. **Admin-driven control of the primary.** The primary is intentionally out of reach of the admin API. To change it, redeploy astro-server with different env vars.

## Schema

PR 1 has landed. This is the data foundation the rest of Phase 1 builds against. See `sql/astro-server/schema.sql`.

### `public.clusters` (additional clusters only)

| Column | Type | Notes |
|---|---|---|
| `id` | `varchar(64)` PK | DNS-safe stable identifier (e.g. `eu-west-1-managed`). Validated by `clusterstore.ValidateID`. |
| `region` | `varchar(64)` | AWS region; placement metadata for operators. |
| `eks_cluster_name` | `varchar(128)` | EKS cluster name; consumed by `aws eks get-token` / the EKS client. |
| `eks_cluster_endpoint` | `varchar` | API server endpoint used by the EKS client. |
| `enabled` | `boolean` default `true` | When `false`, no traffic is routed to this cluster. |
| `created_at` / `updated_at` | `timestamptz` | Standard audit columns. |

The table holds **additional** clusters only. The primary never appears as a row. The schema is deliberately minimal: only the fields needed to construct a working `ClusterClient` and gate traffic. Additional columns (e.g. for cross-account auth or alternative connectivity paths) are added by the milestone that actually needs them.

### `deployments.cluster_id`

Nullable `varchar(64)` with a `FOREIGN KEY ... ON DELETE RESTRICT` to `clusters(id)`.

- **NULL** — deployment runs on the primary cluster. This is the default for every deployment created before per-deployment routing landed and remains the default for any deploy request that doesn't specify a `cluster_id`.
- **Non-NULL** — deployment is pinned to a specific additional cluster. The FK guarantees the row exists.
- **`ON DELETE RESTRICT`** — an additional cluster cannot be deregistered while deployments still reference it; the admin path returns `ErrInUse` and the operator must move or delete those deployments first.

### `internal/clusterstore`

Typed CRUD wrapper around `clusters`. Exposes `Register`, `Get`, `List(enabledOnly)`, `SetEnabled`, `Deregister`, plus `ValidateID`. Maps Postgres errors to `ErrNotFound` / `ErrAlreadyExists` / `ErrInUse` via `errors.As(&pqErr)` + SQLSTATE, consistent with `handlers/knowledge.go` and `handlers/agents.go`. PR 2 does not consume the mutation methods; they exist for the admin API in a later PR.

## Configuration

No new env vars in this phase. The existing primary-cluster env vars stay authoritative:

- `K8S_CLIENT_MODE` — `eks` (default) or `local`.
- `EKS_CLUSTER_NAME` / `K8S_MASTER_URL` / `AWS_REGION` — required in EKS mode; identify the primary cluster.
- `KUBECONFIG` / `KUBE_CONTEXT` — used in local mode.

These are read once in `config.Load()`, validated by `cfg.Validate()`, and passed into the registry's constructor. No second source of truth, no `DEFAULT_CLUSTER_ID`, no backfill insert.

## Runtime Architecture

```
                   ┌─────────────────────────────────────┐
                   │      clusters table (Postgres)      │
                   │      additional clusters only       │
                   └────────────────┬────────────────────┘
                                    │ Register / SetEnabled / Deregister
                                    │ List on admin RPC (future PR)
                                    ▼
┌──────────────┐    ┌────────────────────────────────────────────┐
│ admin gRPC   │───▶│         k8s.Registry  (one per process)    │
│ astro-queen  │    │                                            │
└──────────────┘    │   primary:   ClusterClient (env-var built) │
                    │   additional: map[id]*ClusterClient        │
                    │              (populated in later PR)       │
                    │                                            │
                    │   Default() → primary                      │
                    │   Get(id)   → additional[id]   (future)    │
                    │   List()    → primary + additional (future)│
                    └───────┬────────────────────────┬───────────┘
                            │                        │
              handlers      │                        │  River workers
              (deploy,      │                        │  (deploy, undeploy,
              status,       │                        │   reconcile, wakeup,
              logs, ...)    │                        │   purge, knowledge,
                            │                        │   github_build)
                            ▼                        ▼
              Resolve cluster from         Resolve cluster from job
              deployments.cluster_id        payload.ClusterID
              (NULL → Default)              (empty → Default)
```

### k8s.Registry

The single owner of every `ClusterClient` in the process.

- **Construction** — `NewRegistry` builds the primary `ClusterClient` from `RegistryConfig` (mode + EKS coords + kubeconfig path/context). Boot fails if the primary can't be constructed. Additional-cluster handling lands in later PRs.
- **`Default()`** — returns the primary client. This is the only registry method PR 2 implements; every handler/worker in this PR receives the result of `Default()` exactly where the legacy singleton was used.
- **`Get(ctx, id)`** *(future)* — looks up an additional cluster by id. Returns `ErrClusterNotFound` if no row, `ErrClusterDisabled` if the row's `enabled = false`.
- **`List()`** *(future)* — sorted snapshot of every cluster the registry knows about, including a synthesized entry for the primary so admin tooling can show the complete view.
- **`Refresh(ctx, id)`** *(future)* — re-reads a single additional row from `clusterstore` and updates the cache. Called by admin lifecycle RPCs.

The registry is constructed in `main.go` and passed into `setupRoutes` and `runWorker`. Until per-deployment routing lands, every consumer just calls `registry.Default()`.

### Why no backfill

The earlier design materialised the primary as a row in `clusters` on first boot. That created two sources of truth for the same value (env vars + table) and required the registry to upsert on boot. The current design treats env vars as the authority for the primary and the table as the authority for additional clusters — one source of truth per cluster, no synchronisation, no boot-time DB write.

The cost is one structural asymmetry: the primary has no row, so future admin APIs can't disable or rename it. That's intentional — changing the primary is a deployment artifact concern (Helm values, Terraform, env vars), not a runtime concern.

## Request Plumbing (delivered across subsequent PRs)

The deploy request gains an optional `cluster_id`. The lifecycle of a deployment becomes:

1. **Create / redeploy.** The deploy handler reads `cluster_id` from the request body. If absent, leaves the column `NULL` (= primary). If present, validates via `clusterstore.Get` and rejects with 400 if the row is missing or disabled.
2. **Enqueue.** Every River job constructor (deploy, undeploy, reconcile, wakeup, purge, knowledge_reconcile, github_build, privatelink_provision when it touches K8s) copies `cluster_id` from the deployment row into the job payload. NULL becomes empty.
3. **Worker execution.** The worker reads `args.ClusterID`. Empty/NULL → `registry.Default()` (primary). Non-empty → `registry.Get(args.ClusterID)` (additional cluster).
4. **Handler reads.** Every handler that touches K8s state (status, logs, events, restart, config/secret introspection, knowledge store CRUD, github-build logs) resolves its cluster by looking up the deployment's `cluster_id` first (via `deploymentstore.GetClusterID`). NULL → primary; non-null → additional via `registry.Get`.

PR 2 implements **only** the registry's primary path. The `cluster_id` column exists on `deployments` (added in PR 1) but no caller reads or writes it yet. Per-deployment routing is the work of subsequent PRs.

Probe handlers and the `Status` admin RPC will eventually aggregate health across the primary and every enabled additional cluster. PR 2 retains today's single-cluster probe semantics (primary only).

### Disabled additional clusters

`enabled = false` is the gate between "registered" and "accepting work":

- The registry does not construct or cache a `ClusterClient` for a disabled additional row.
- The deploy path rejects requests with `cluster_id` pointing at a disabled row.
- In-flight jobs whose `ClusterID` points at a row that becomes disabled after enqueue surface a clear error from `registry.Get` and follow the worker's normal retry/dead-letter path. There is no automatic re-routing.
- Disabled rows appear in `ListClusters` and `Status` so operators can see staged or quarantined additional clusters.

The primary has no `enabled` flag. Taking the primary offline is a redeploy.

## Admin API (later PR)

Adds five RPCs to `apps/astro-server/internal/admingrpc` (protobufs in `packages/astro-proto`), targeting *additional* clusters only:

- `RegisterCluster(RegisterClusterRequest) → Cluster` — `clusterstore.Register` + `registry.Refresh(id)`.
- `EnableCluster(ClusterRef) → Cluster` — `SetEnabled(id, true)` + `Refresh`.
- `DisableCluster(ClusterRef) → Cluster` — `SetEnabled(id, false)` + `Refresh` (drops the client from the cache).
- `DeregisterCluster(ClusterRef) → Empty` — `Deregister`; returns `FailedPrecondition` if the FK from `deployments` still references the row.
- `ListClusters(ListClustersRequest) → ListClustersResponse` — pass-through over `clusterstore.List` + synthesised primary entry; optional `enabled_only` filter (the primary is always treated as enabled).

The existing `Status` RPC is extended with `per_cluster_health: map<cluster_id, CheckResult>` computed from the primary + `registry.List() + ClusterClient.CheckHealth()`.

None of these RPCs operate on the primary. Re-targeting a deployment requires a redeploy with an explicit `cluster_id`. This keeps the deploy spec the single source of truth for "where does this deployment run."

## Operator Surface (later PR)

astro-queen gains a `Clusters` view backed by `ListClusters` + `Status.per_cluster_health`:

- List shows id, region, enabled, healthy. The primary is shown alongside additional clusters but flagged as such (no enable/disable/deregister actions).
- Key bindings (additional clusters only): `r` register (form: id, region, eks_cluster_name, endpoint), `e` enable, `d` disable, `D` deregister.
- Deregister confirmation uses a Bubbletea modal — no native browser/terminal alert, per the user-rule.

No web-client UI is in scope for Phase 1. Operators run astro-queen against the target environment.

## Migration

No data migration. The cutover is:

1. **Schema** (already landed in PR 1) — new `clusters` table and nullable `deployments.cluster_id` column. No code reads from them yet.
2. **Registry (this PR)** — astro-server boots, constructs the primary `ClusterClient` from env vars, and every existing handler/worker receives `registry.Default()`. Behaviour is identical to the legacy singleton.
3. **Per-request plumbing rollout** — deploy payload, deployment row, worker payloads, then handlers migrate to per-cluster resolution in sequence. Each step preserves `Default()` as the primary path so partially-migrated environments stay correct.
4. **Admin API + TUI** — operators can register / enable / disable / deregister *additional* clusters at runtime.
5. **Acceptance** — register a fake `test-disabled` additional cluster in preview, smoke-test every flow against the primary, confirm the disabled row is visible but never receives work, tear it down, write the acceptance changelog.

Rolling back any step in this sequence leaves the system in a working primary-only state because every code path retains `Default()` as the fallback. There is no flag-day.

### Side effects

- New table and column in production Postgres (already applied via Atlas in PR 1).
- **No new rows in `clusters` on boot.** The table stays empty until operators register additional clusters via the admin API.
- `Status` admin RPC response shape will gain `per_cluster_health` in a later PR; existing fields unchanged.
- River job payloads will gain an optional `ClusterID` field in a later PR. Empty values are tolerated and resolved to the primary.

## What This Kills

- The singleton `k8s.ClusterClient` shared across `main.go`, every handler, and every worker. Replaced by `k8s.Registry`.
- The "implicit cluster" assumption baked into deploy, undeploy, reconcile, wakeup, purge, knowledge-reconcile, and github-build workers — replaced by explicit primary-or-additional resolution.
- The "one cluster's health is the server's health" probe semantics. Probes still report the primary for compatibility, but a future PR aggregates across additional clusters too.

## What This Preserves

- Existing single-cluster topology and credentials. No change to `aws eks get-token`, IRSA, or the EKS client implementation.
- The `ClusterClient` interface and its `EKSClient` / `LocalClient` implementations.
- All existing handler and worker external behavior when `cluster_id` is unset.
- Atlas-managed schema evolution.
- Existing audit-log and observability touchpoints; new audit events for cluster lifecycle RPCs slot into the same table.

## Open Questions

1. **Promoting an additional cluster to be the primary.** Today the only way to change the primary is to redeploy astro-server with new env vars. If an operator wants to swap which region is the home cluster without a full redeploy, that's a future operator workflow worth designing. Not blocking Phase 1.
2. **Cross-cluster admin reads.** Listing deployments across every cluster in one call is not exposed today; every read is scoped by deployment row → cluster row. A future admin "fan out across primary + all enabled additionals" path is plausible if needed.
3. **Worker fallback log noise.** A long-tail of pre-cutover queued jobs will hit the `ClusterID == ""` fallback after deploy and route to the primary. Likely emit at `INFO` rather than `WARN` because primary-routing is the steady-state for unrouted deployments, not a regression.
