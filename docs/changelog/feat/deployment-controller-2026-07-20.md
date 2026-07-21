# Deployment controller — Phase 1 (observe + persist)

## Summary

First slice of the event-driven deployment controller from
`docs/plans/deployment-state-tracking.md`. It replaces per-request K8s polling
and the deleted reconcile loop with a long-running controller that watches
managed workloads via informers and persists their observed health to a new
`deployment_workload_status` table.

Phase 1 is **shadow mode**: it observes and persists only. It does not yet drive
the deployment-level lifecycle (`deployments.status`) — that is Phase 2. This
lets us validate the observed data against reality before anything depends on it.

## Design

**Where it runs.** The controller lives in the single-replica `astro-worker`
process (started from `runWorker`, wired to the worker context so it shuts down
with the process). A single writer means no leader election is needed. Because
the multi-replica API cannot see the worker's in-memory informer caches, the
controller persists to Postgres and the API reads from there.

**Watch → queue → sync.** Per cluster (discovered via `k8s.Registry.List`,
re-polled to pick up clusters added at runtime), a `SharedInformerFactory`
watches Deployments, StatefulSets, Jobs, CronJobs, and Pods, scoped by the
`app.kubernetes.io/managed-by=astro-server` label. Event handlers enqueue a
`(cluster, namespace)` key into a rate-limited workqueue — bursty per-object
events coalesce to one unit of work. Sync workers read the full workload set for
that namespace from the informer caches (no live API calls), derive each
workload's health, and replace the deployment's rows in one transaction. The
informer resync interval re-derives everything periodically — the self-healing
backstop that replaced the old reconcile job.

**Health from K8s rollout signals.** Per-workload phase
(`pending`/`progressing`/`ready`/`complete`/`failed`) is derived from the
workload object's own status — `observedGeneration` vs generation, updated /
available / ready replicas, StatefulSet revisions, Job conditions, and a
Deployment `Progressing=ProgressDeadlineExceeded` condition as a deterministic
failure signal. Pod-level failure-reason enrichment (ImagePullBackOff, etc.) and
folding this into the deployment lifecycle are deferred to Phase 2.

**Persistence.** New `deployment_workload_status` table (one row per
`(deployment, workload)`): phase, reason, message, observed ready/desired,
observed generation, observed-at. `ReplaceWorkloadStatuses` rewrites the set per
sync so removed workloads are pruned; `GetWorkloadStatuses` is the read path.

## Migration

Additive — the `deployment_workload_status` table is applied by Atlas from the
declarative schema (`sql/astro-server/schema.sql`); no data migration. No config
or API changes. The controller only starts when a K8s registry is configured, so
local/registry-less environments are unaffected.
