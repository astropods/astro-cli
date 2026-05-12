# Multi-Region Cluster Support — Astro-Server Contract

**Version**: 1.0
**Status**: Draft
**Date**: 2026-05-12
**Plan**: [m0_phase_1_pr_sequence_60a44a07.plan.md](../../../../.cursor/plans/m0_phase_1_pr_sequence_60a44a07.plan.md)

## Overview

`astro-server` today reconciles workloads into exactly one managed Kubernetes cluster. The cluster identity is a global singleton — chosen at boot from `EKS_CLUSTER_NAME` / `K8S_MASTER_URL` env vars and held as a single `k8s.ClusterClient` shared by every handler and River worker.

This spec defines the astro-server-side contract for running against **N managed clusters from a single control plane**. After this work, every deployment is bound to a specific cluster row at creation time, every code path that touches Kubernetes resolves its client from that row, and operators can register/enable/disable/deregister clusters at runtime without redeploying astro-server.

This is **Milestone 0, Phase 1**. It changes the application contract only. No new infrastructure is provisioned and the default single-cluster topology continues to run unmodified in production. Subsequent phases provision additional clusters and validate cross-region behavior on top of this contract.

## Terminology

- **Cluster** — an EKS (or local-dev) Kubernetes cluster that astro-server can reconcile agent workloads into. Identified by a stable string `id` (e.g. `us-east-1-managed`).
- **Cluster row** — a row in `public.clusters` describing one cluster: identity, region, EKS coordinates, enabled flag.
- **ClusterClient** — existing Go interface in `internal/k8s` that abstracts Kubernetes connectivity (`Clientset`, `Config`, `CheckHealth`, `GetServerVersion`, `DiagnoseConnection`).
- **Registry** — new in-memory map of `cluster_id → ClusterClient`. The single owner of all `ClusterClient` instances at runtime. Replaces the singleton `k8sClient` in `main.go`.
- **Default cluster** — the cluster id used when a deployment or job has no explicit `cluster_id`. Configured by `DEFAULT_CLUSTER_ID`. Provides backwards compatibility for existing deployments and operators that haven't started selecting clusters yet.
- **Backfill** — the one-time, idempotent insert on first boot under this contract that materialises a single cluster row from the existing env vars so production keeps working with no operator action.
- **Disabled cluster** — a cluster row with `enabled = false`. Registered and visible to operators, but the registry will not hand out a client for it and the deploy path rejects requests targeting it.

## Goals

1. **Cluster as a first-class entity.** Replace the implicit "the cluster" with explicit `cluster_id` references in the schema, deploy payload, worker payloads, and admin RPCs.
2. **One owner of K8s clients at runtime.** The registry is the only place that constructs `ClusterClient`s. Handlers and workers receive it (or resolve specific entries from it); they never call `NewClusterClient` themselves.
3. **Single-cluster behavior unchanged.** Production with one cluster behaves identically before and after this work. The default-cluster fallback covers every legacy code path that didn't pass a `cluster_id`.
4. **Operator-managed lifecycle.** Clusters are registered, enabled, disabled, and deregistered through the admin gRPC API and astro-queen TUI — without restarting astro-server. New rows become reachable on the next `Refresh`; disabled rows stop accepting work immediately.
5. **No silent fallback in production.** Workers and handlers log clearly when they fall back to the default cluster, so operators see the legacy path in use during the migration window.
6. **Stage-before-promote workflow.** A new cluster can be registered with `enabled = false`, smoke-tested via admin RPCs, and only then enabled for real traffic.

## Non-Goals

1. **Cross-account / cross-VPC connectivity.** Every cluster is assumed reachable using astro-server's existing IAM identity (same AWS account, same auth path as today). The `sts:AssumeRole` story for customer-managed clusters is a later milestone.
2. **Re-routing existing deployments.** Deployments that exist at cutover have `cluster_id = NULL` and continue to be served by the default cluster. There is no bulk migration of `cluster_id` values; re-targeting a deployment requires a redeploy.
3. **Cluster auto-discovery.** Registration is an explicit operator action via admin gRPC / astro-queen. The server does not enumerate EKS clusters in the AWS account.
4. **Web UI for cluster selection on the deploy form.** The deploy payload accepts an optional `cluster_id` for API callers; the astro-client UX for choosing it is a separate frontend effort.
5. **Infrastructure provisioning.** This spec covers only the astro-server application contract. Terraform module changes, IAM/IRSA generalisation, and bringing up additional clusters are handled by separate work that consumes this contract.

## Schema

PR 1 has landed. This is the data foundation the rest of Phase 1 builds against. See `sql/astro-server/schema.sql`.

### `public.clusters`

| Column | Type | Notes |
|---|---|---|
| `id` | `varchar(64)` PK | DNS-safe stable identifier (e.g. `us-east-1-managed`). Validated by `clusterstore.ValidateID`. |
| `region` | `varchar(64)` | AWS region; placement metadata for operators. |
| `eks_cluster_name` | `varchar(128)` | EKS cluster name; consumed by `aws eks get-token` / the EKS client. |
| `eks_cluster_endpoint` | `varchar` | API server endpoint used by the EKS client. |
| `enabled` | `boolean` default `true` | When `false`, no traffic is routed to this cluster. |
| `created_at` / `updated_at` | `timestamptz` | Standard audit columns. |

The schema is deliberately minimal: only the fields needed to construct a working `ClusterClient` and gate traffic. Additional columns (e.g. for cross-account auth or alternative connectivity paths) are added by the milestone that actually needs them.

### `deployments.cluster_id`

Nullable `varchar(64)` with a `FOREIGN KEY ... ON DELETE RESTRICT` to `clusters(id)`.

- **NULL** on existing rows means "no cluster recorded"; the read path resolves this to `DEFAULT_CLUSTER_ID`.
- **RESTRICT** ensures a cluster cannot be deregistered while deployments still reference it; the admin path returns `ErrInUse` and the operator must move or delete those deployments first.

### `internal/clusterstore`

Typed CRUD wrapper around `clusters`. Exposes `Register`, `Get`, `List(enabledOnly)`, `SetEnabled`, `Deregister`, plus `ValidateID`. Maps Postgres errors to `ErrNotFound` / `ErrAlreadyExists` / `ErrInUse` via `errors.As(&pqErr)` + SQLSTATE, consistent with `handlers/knowledge.go` and `handlers/agents.go`.

## Configuration

One new setting on `cfg.Deployment`:

- **`DEFAULT_CLUSTER_ID`** — id of the cluster used when a deploy request, deployment row, or worker job has no explicit `cluster_id`. If unset at startup, derived from `EKS_CLUSTER_NAME` (lower-cased, normalised to match `idPattern`).

Existing env vars (`EKS_CLUSTER_NAME`, `K8S_MASTER_URL`, `AWS_REGION`, `K8S_CLIENT_MODE`) become **bootstrap-only inputs** to the backfill flow. After backfill they are still read for new astro-server boots that haven't run before, but the runtime path reads cluster data from the `clusters` table via the registry. They are not removed in Phase 1; their removal happens once every environment has been observed running off the registry for a stable window.

## Runtime Architecture

```
                          ┌─────────────────────────────────────┐
                          │      clusters table (Postgres)      │
                          └────────────────┬────────────────────┘
                                           │ Register / SetEnabled / Deregister
                                           │ List on boot + Refresh on admin RPC
                                           ▼
┌──────────────┐   ┌────────────────────────────────────────────────────────┐
│ admin gRPC   │──▶│         k8s.Registry  (one per astro-server)           │
│ astro-queen  │   │                                                        │
└──────────────┘   │   map[string]*ClusterClient   default: <DEFAULT_ID>    │
                   │   Get(ctx, id)   Default()   List()   Refresh(ctx,id)  │
                   └───────┬──────────────────────────────────┬─────────────┘
                           │                                  │
              handlers/    │                                  │   River workers
              (deploy,     │                                  │   (deploy, undeploy,
              status,      │                                  │    reconcile, wakeup,
              logs, ...)   │                                  │    purge, knowledge,
                           │                                  │    github_build)
                           ▼                                  ▼
                  Resolve cluster from           Resolve cluster from job
                  deployments.cluster_id          payload.ClusterID
                  (NULL → Default)                (empty → Default + log)
```

### k8s.Registry

New type in `internal/k8s`. Owns the lifecycle of every `ClusterClient` in the process.

- **Construction** — on startup, `List(enabledOnly=true)` from `clusterstore` and build a `ClusterClient` per row via the existing `NewClusterClient` factory. If `clusters` is empty, run the backfill insert first (see below), then construct.
- **`Get(ctx, id)`** — return the client for `id`. Errors if `id` is unknown to the registry or its row is disabled.
- **`Default()`** — return the client for `DEFAULT_CLUSTER_ID`. The only path allowed to skip explicit lookup.
- **`List()`** — slice of `(id, region, ClusterClient, healthy bool)` for status pages and admin RPCs.
- **`Refresh(ctx, id)`** — re-read the row from `clusterstore` and add / replace / drop the cached entry. Used by `RegisterCluster` / `EnableCluster` / `DisableCluster` / `DeregisterCluster` to pick up changes without an astro-server restart.

The registry is constructed in `main.go` and passed into `setupRoutes` and `runWorker`. Handlers and workers receive either the registry itself (when they need to resolve per-request) or `registry.Default()` (transitionally, until they migrate to per-deployment resolution).

### Backfill

On first boot under this contract, if `clusters` is empty, the registry inserts one row built from `EKS_CLUSTER_NAME` / `K8S_MASTER_URL` / `AWS_REGION` and the derived `DEFAULT_CLUSTER_ID`. The insert is idempotent (`ON CONFLICT DO NOTHING`) and runs inside the same transaction that observes the empty table. Subsequent boots see the row and skip backfill. Operators may delete the backfilled row only after replacing it via `RegisterCluster`; the FK from `deployments.cluster_id` enforces this in the common path.

## Request Plumbing

The deploy request gains an optional `cluster_id`. The lifecycle of a deployment becomes:

1. **Create / redeploy.** The deploy handler reads `cluster_id` from the request body. If absent, fills it from `DEFAULT_CLUSTER_ID`. If present, validates via `clusterstore.Get` and rejects with 400 if the row is missing or disabled. Persists to `deployments.cluster_id`.
2. **Enqueue.** Every River job constructor (deploy, undeploy, reconcile, wakeup, purge, knowledge_reconcile, github_build, privatelink_provision when it touches K8s) copies `cluster_id` from the deployment row into the job payload.
3. **Worker execution.** The worker reads `args.ClusterID`, resolves a `ClusterClient` via `registry.Get`. Empty `ClusterID` falls back to `registry.Default()` and emits a `WARN`-level log line so operators can spot the legacy path during the migration window.
4. **Handler reads.** Every handler that touches K8s state (status, logs, events, restart, config/secret introspection, knowledge store CRUD, github-build logs) resolves its cluster by looking up the deployment's `cluster_id` first (via `deploymentstore.GetClusterID`) and then `registry.Get`. NULL maps to `Default()` for backward compatibility.

Probe handlers and the `Status` admin RPC aggregate health across **all enabled clusters** via `registry.List() + CheckHealth`. The legacy single-cluster probe stays as-is for compatibility.

### Disabled clusters

`enabled = false` is the gate between "registered" and "accepting work":

- The registry does not construct or cache a `ClusterClient` for a disabled row.
- The deploy path rejects requests with `cluster_id` pointing at a disabled row.
- In-flight jobs whose `ClusterID` points at a row that becomes disabled after enqueue surface a clear error from `registry.Get` and follow the worker's normal retry/dead-letter path. There is no automatic re-routing.
- Disabled rows appear in `ListClusters` and `Status` so operators can see staged or quarantined clusters.

## Admin API

Adds five RPCs to `apps/astro-server/internal/admingrpc` (protobufs in `packages/astro-proto`):

- `RegisterCluster(RegisterClusterRequest) → Cluster` — `clusterstore.Register` + `registry.Refresh(id)`.
- `EnableCluster(ClusterRef) → Cluster` — `SetEnabled(id, true)` + `Refresh`.
- `DisableCluster(ClusterRef) → Cluster` — `SetEnabled(id, false)` + `Refresh` (drops the client from the cache).
- `DeregisterCluster(ClusterRef) → Empty` — `Deregister`; returns `FailedPrecondition` if the FK from `deployments` still references the row.
- `ListClusters(ListClustersRequest) → ListClustersResponse` — pass-through over `clusterstore.List`; optional `enabled_only` filter.

The existing `Status` RPC is extended with `per_cluster_health: map<cluster_id, CheckResult>` computed from `registry.List() + ClusterClient.CheckHealth()`.

There is no imperative API for editing `deployments.cluster_id` out-of-band. Re-targeting a deployment requires a redeploy with an explicit `cluster_id`. This keeps the deploy spec the single source of truth for "where does this deployment run."

## Operator Surface

astro-queen gains a `Clusters` view backed by `ListClusters` + `Status.per_cluster_health`:

- List shows id, region, enabled, healthy.
- Key bindings: `r` register (form: id, name, region, eks_cluster_name, endpoint), `e` enable, `d` disable, `D` deregister.
- Deregister confirmation uses a Bubbletea modal — no native browser/terminal alert, per the user-rule.

No web-client UI is in scope for Phase 1. Operators run astro-queen against the target environment.

## Migration

No data migration. The cutover is:

1. **Schema** (already landed) — new `clusters` table and nullable `deployments.cluster_id` column. No code reads from them yet.
2. **Registry + backfill** — astro-server boots, sees an empty `clusters` table, inserts the legacy env-var row, then constructs its registry. Every existing handler/worker still receives `registry.Default()` and observes identical behavior.
3. **Per-request plumbing rollout** — deploy payload, deployment row, worker payloads, then handlers migrate to per-cluster resolution in sequence. Each step preserves `Default()` as the fallback so partially-migrated environments stay correct.
4. **Admin API + TUI** — operators can now register/enable/disable/deregister clusters at runtime.
5. **Acceptance** — register a fake `test-disabled` row in preview, smoke-test every flow against the default cluster, confirm the disabled row is visible but never receives work, tear it down, write the acceptance changelog.

Rolling back any step in this sequence leaves the system in a working single-cluster state because every path retains the default fallback. There is no flag-day.

### Side effects

- New table and column in production Postgres (already applied via Atlas in PR 1).
- One new row in `clusters` per environment on first boot under PR 2.
- `Status` admin RPC response shape gains `per_cluster_health`; existing fields unchanged.
- River job payloads gain an optional `ClusterID` field. Empty values are tolerated and resolved to default; in-flight pre-cutover jobs are unaffected.

## What This Kills

- The singleton `k8s.ClusterClient` shared across `main.go`, every handler, and every worker. Replaced by `k8s.Registry`.
- Direct reads of `EKS_CLUSTER_NAME` / `K8S_MASTER_URL` outside of registry construction and backfill.
- The "implicit cluster" assumption baked into deploy, undeploy, reconcile, wakeup, purge, knowledge-reconcile, and github-build workers.
- The "one cluster's health is the server's health" probe semantics. Probes still report the default for compatibility, but the authoritative answer is per-cluster.

## What This Preserves

- Existing single-cluster topology and credentials. No change to `aws eks get-token`, IRSA, or the EKS client implementation.
- The `ClusterClient` interface and its `EKSClient` / `LocalClient` implementations.
- All existing handler and worker external behavior when `cluster_id` is unset or matches the default.
- Atlas-managed schema evolution.
- Existing audit-log and observability touchpoints; new audit events for `RegisterCluster` / `EnableCluster` / `DisableCluster` / `DeregisterCluster` slot into the same table.

## Open Questions

1. **Worker fallback log noise.** A long-tail of pre-cutover queued jobs will hit the `ClusterID == ""` fallback after deploy. Default is `WARN`; if volume is high we may downgrade once the queue drains, or add a one-shot backfill on the queue table.
2. **Changing the default cluster at runtime.** Deregistering the default cluster is blocked by the FK while deployments still reference it, and the operator must reconfigure `DEFAULT_CLUSTER_ID` before deregistering. A dedicated `SetDefaultCluster` RPC may be worth adding once a second cluster exists in production.
3. **Cross-cluster admin reads.** Listing deployments across every cluster in one call is not exposed today; every read is scoped by deployment row → cluster row. A future admin "fan out across all enabled clusters" path is plausible if needed.
