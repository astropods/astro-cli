# Deployment runtime read-model

## Summary

`GET /deployments/:id/runtime` hit the Kubernetes API on every request — listing
workloads and pods, building per-container status, and doing a Service GET for
messaging reachability — once per viewer, per poll. This makes the endpoint
DB-backed, extending the same "controller observes, API reads the DB" pattern
already used for deployment status: the event-driven controller persists a full
live-runtime snapshot from its informer caches, and the endpoint reads that
instead of the cluster.

## Design

**One JSONB document per deployment.** New `deployment_runtime_status` table
holds the whole runtime view as a single `snapshot` document. The view is read
whole and rendered whole — never queried by an inner field — so a document is
the right shape, not normalized tables: every workload, all of its pods, all of
their containers, and all managed services are captured, and new pods /
containers / services are just more JSON, never a schema change.

**Controller assembles it from caches it already has.** The controller already
watches Deployments, StatefulSets, Jobs, CronJobs, and Pods; it gains a Services
informer. On each sync it builds the snapshot and upserts it. Writes are
content-diffed (an unchanged steady-state deployment doesn't rewrite the row),
and an empty snapshot (torn-down namespace) clears the row and evicts its
diff-cache entry.

**The endpoint projects the snapshot to the unchanged response.** `/runtime`
deserializes the document and maps it to the existing `DeploymentRuntime` shape —
representative pod per workload (Running first, then newest), `messaging_reachable`
derived from the messaging Service plus the sidecar container's readiness. The
response shape is unchanged; the handler drops its now-unused K8s registry /
agent-index / cache parameters.

**Deletes the live-enrichment pipeline it replaced.** `/runtime` was the only
caller of `enrichDeployment` + `listAstroDeployments` and their helpers
(per-container status building, pod selection, job/cronjob status, manual-
ingestion annotation parsing). The deployments *list* endpoint already reads
pure DB (`agentDeploymentFromDB`), so with `/runtime` converted this whole path
became dead — ~1,000 lines removed (incl. tests). The only K8s-live reads left
in the deployment path are ones that must be live: mutations (restart), pod
logs, and the K8s Events feed.

## Related status cleanup (client + server)

Folded in from the deployment-status wiring work:

- **Status toggle handles failure.** `AgentStatusToggle` had no `error` branch, so
  a failed deploy (non-stopped record status) rendered a healthy green "Active".
  It now shows a red "Error" state, suppressed during an optimistic pause/resume.
- **Dead status contract removed end-to-end.** `ready_lag` / `cluster_unreachable`
  reasons and `status_changed_at` are gone from the client types, the
  `deriveChatComposerState` branch, and the server `/status` response body (the
  DB column stays; billing/admin still use it). Nothing consumed them after
  status became controller-driven.
- **Stale runtime doc comments corrected** — the client no longer describes
  `/runtime` as a "live K8s read" that "can 503"; it's the observed,
  cluster-independent read described above.

## Behavior change

`/runtime` is now cluster-independent: a disabled or briefly unreachable cluster
returns `200` with the last-observed snapshot (or an empty runtime, which the UI
already renders as "loading"), where it previously returned `503`. There is no
live fallback — a deployment the controller hasn't observed yet reads empty
until its first sync.

## Scope

- **Not moved:** `manual_ingestions` still derives from a namespace annotation
  (a pre-existing item slated to move to the DB). The field is carried in the
  snapshot shape but not yet populated by the controller.
- **Still deferred:** controller HA / leader election. The controller is the
  sole writer of this read-model, so a controller outage freezes the runtime
  view — acceptable while it runs single-replica in astro-worker.

## Migration

Schema only — adds `deployment_runtime_status` via the standard schema apply. No
config or public API changes.
