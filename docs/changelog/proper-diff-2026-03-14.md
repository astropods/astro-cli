# Async database-driven deployment architecture

## Summary

Deploy and undeploy handlers synchronously called K8s APIs inline with the HTTP request, tying up connections for the full provisioning duration, providing no status visibility, conflicting with KEDA-scaled-to-zero namespaces, and offering no retry path on partial failure. This change decouples the API from K8s: handlers write desired state to the database and enqueue River jobs, workers reconcile DB state to K8s asynchronously, and a unified reconciler replaces both drift check and namespace scan with KEDA awareness.

## Design

Handlers now return `202 Accepted` with a deployment ID. A status state machine tracks each deployment through `pending → provisioning → active` (or `→ failed`), with additional states for `undeploying → undeployed` and `scaled_down` (KEDA). Every status transition inserts an audit event into `deployment_events`.

A `deployment_revisions` table stores versioned spec snapshots. Each deploy/redeploy creates a new revision rather than overwriting the spec. The `deployments.current_revision` column points to the active revision, enabling rollback by simply setting `current_revision` to a previous value and enqueuing a deploy job.

The `deployer` package extracts K8s apply/teardown from the handler into a reusable struct shared by four River workers: `DeployWorker` (provisions resources), `UndeployWorker` (tears down namespace), `WakeUpWorker` (re-provisions KEDA-scaled-down deployments), and `ReconcileWorker` (replaces driftcheck + nsscan). The reconciler detects KEDA scale-downs via the ScaledObject `Active` condition, performs drift auto-remediation with an `astro.dev/reconcile=paused` annotation opt-out, detects stale provisioning and stuck pending deployments, and maintains namespace ownership.

New endpoints: `GET /deployments/:id/status` (status + events + revisions), `POST /deployments/:id/wakeup` (KEDA recovery), `POST /deployments/:id/rollback` (revert to previous revision).

The `deployment_env_vars` table is dropped — it was write-only in production (resolved secret values stored unnecessarily). Variables remain in `deployment_variables` with KMS envelope encryption.

## Migration

No user-facing breaking changes. The deploy/undeploy CLI commands receive `202` instead of `200` — any 2xx should be treated as success. Existing `active`/`undeployed` DB rows remain valid. Schema changes are additive (new columns with defaults, new tables). The reconciler subsumes drift check and namespace scan with no configuration changes.
