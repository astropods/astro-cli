# Fix deploys silently stuck on missing images

## Summary

A deployment sat in `Deploying` for over half an hour because Kubernetes was looping on `ImagePullBackOff` for a tag that did not exist in the registry, and nothing in the system surfaced that fact: the deploy API accepted the request, the reconcile loop never escalated the pod failure into the deployment row, the list API mapped the half-ready ReplicaSet to `Deploying`, and the dashboard rendered `Deploying` indefinitely. The redeploy path also pinned the previously-deployed build (intended behavior), so re-issuing the command did not pick up newer pushes — and the UI offered no obvious way to upgrade.

This change makes deploys fail fast when the image is missing, propagates real pod failures to deployment status, surfaces the failure reason in the UI, and gives users a one-click affordance to redeploy with the latest build.

## Design

### Fail-fast image preflight (server)

Added `k8s.ImagePreflighter` in `apps/astro-server/internal/k8s` that performs a registry HEAD against `/v2/<repo>/manifests/<tag>` with the manifest accept header, caches positive results, and treats 404 — plus 5xx from the local mirror host — as `ErrImageNotFound`. The preflighter is constructed once in `main.go` and threaded through three call sites:

- `handlers.DeployAgent` — synchronous preflight before enqueueing the river job; on `ErrImageNotFound` it returns HTTP 422 with `{error, image}` so the CLI can show an actionable message instead of letting the deployment enter the queue.
- `riverqueue.Config` → `deployer.Deployer` → `k8s.Applier` — the same preflighter is reused inside `resolveContainerImage` so reconcile-driven applies (e.g. spec rebases) cannot resurrect a deployment with a missing image.

`ApplierConfig` gained `TenantImageHosts` so we only preflight images we own; third-party base images are skipped.

### Pod-failure escalation (server)

`riverqueue.ReconcileWorker` now scans pods owned by the deployment and classifies waiting-state reasons (`ImagePullBackOff`, `ErrImagePull`, `InvalidImageName`, `CreateContainerConfigError`, etc.) plus `CrashLoopBackOff` once it crosses a restart threshold. Fresh pods (younger than `podFailureWaitGrace`) are ignored to avoid flapping during normal startup. Persistent failures are written to the deployment row as `StatusFailed` with a human-readable `error_message` derived from the pod status, and an event is appended in the same transaction. Already-failed deployments are skipped — escalation is idempotent.

### API status truth-from-DB (server)

The list/get deployment handlers used to derive status purely from K8s replica counts, which masks DB-known failures. `agentDeploymentFromDB` and `applyDBFields` now treat `StatusFailed` as authoritative: the JSON response carries `status: "error"` and the new `error_message` field regardless of what the live ReplicaSet reports. The list handler also batch-loads the latest `agent_versions` row per (account, agent) and returns `latest_build_id` so the dashboard does not need N+1 blueprint queries to know whether a newer build exists.

### UI: error tooltip + one-click upgrade (client)

`DeploymentStatusBadge` accepts an `errorMessage` prop. When status is `error`, the badge wraps in a Radix tooltip showing the server message — so a user staring at a red `Error` badge can hover and read `Image registry.local/...:abc123 not found` immediately.

`DeployedAgentCard` consumes the new server-supplied `latest_build_id`. The "Update available" indicator is now a button: clicking it opens a confirmation dialog (no native `confirm()`), and on confirm routes to the deployment detail page with `autoConfigureNewBuild: true` in router state. `ActiveDetailView` reads that flag through a `useRef`-guarded effect and opens the existing configure panel pre-set to "new build" mode. The full configure flow (validation, variable diffs, secret resolution) is unchanged — this is only a discovery hoist.

### CLI: distinguish "build not found" from "blueprint not found"

The `/deployment-template` endpoint returns 404 for three different reasons (unknown account, unknown blueprint, blueprint exists but the requested build does not). `ast deploy` flattened all three into `blueprint %q not found`, which was actively misleading when a user passed `--build <id>` for a build that had not been pushed. `notFoundFromTemplateErr` now parses the response body and emits a targeted message for each case, with the legacy text as a fallback for unknown bodies.

## Migration

No migration required. All changes are server- and client-side; existing deployment rows continue to work and the new `error_message` / `latest_build_id` fields are optional in API responses.
