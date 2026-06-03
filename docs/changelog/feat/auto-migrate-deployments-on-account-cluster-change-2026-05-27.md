# Auto-migrate deployments on account cluster change

## Summary

Changing an account's cluster pin via admin `SetAccountCluster` previously updated only `accounts.cluster_id`. Existing deployments kept routing to their old cluster until manual redeploy or Queen re-apply — and re-apply did not tear down the source namespace, leaving orphaned workloads on cross-cluster moves.

This branch adds automatic per-deployment migration when the account cluster changes, a River worker to run teardown → routing update → redeploy safely, admin APIs and Queen UI to operate and observe migrations, and tighter error handling when enqueue and account update diverge.

**Preview deploy CI:** `deploy-preview.yml` is unchanged from `main` (no `kubectl rollout restart` or other workflow edits on this branch).

## Design

### Account cluster change → queued migrations

- **`SetAccountCluster`** lists active, failed, scaled_down, and **pending** deployments whose routing cluster differs from the new account target, enqueues one `migrate_deployment_cluster` River job per deployment **before** updating the account row, then updates `accounts.cluster_id`. Enqueue and account UPDATE are intentionally separate (workers use job args, not the account row).
- Response fields: `migrations_enqueued`, `deployment_ids`.
- Partial enqueue failures and `SetClusterID` failures after successful enqueues return explicit errors (counts + “retry SetAccountCluster, avoid ReapplyDeployment until complete”) so ops know migrations may already be in flight.

### `MigrateDeploymentClusterWorker` and `clusterplacement.Migrator`

- Worker runs **`clusterplacement.Migrator.MigrateDeployment`**: teardown on the **source** cluster (`TeardownOnCluster`, tolerates `ErrClusterClientUnavailable` like undeploy), then **`ApplyClusterMigration`** — a single DB transaction with a `WHERE status = prior` guard on the routing update, migration event, scaled-down clear, and pending transition — then enqueue deploy.
- Concurrent status changes (e.g. undeploy) abort the transaction and skip. If routing is already at the target but status is still pending, retries finish by enqueuing deploy only; stuck pending is also picked up by the reconciler.
- **`ReapplyDeployment`** with a placement mismatch enqueues the same migration job instead of updating routing inline without teardown.

### Admin gRPC: `ListClusterMigrations`

- New RPC returns recent migration-related deployment events and River jobs, plus a live **placement mismatch** count (deployments whose routing `cluster_id` differs from the account pin).
- Events match fixed prefixes from `clusterplacement.MigrationEventMessage` / `AccountMigrationEventMessage` (and admin re-apply wording) — not broad ILIKE.
- Jobs: all `migrate_deployment_cluster` rows; `deploy` only when enqueued within 2h after a migrate for the same deployment (avoids routine deploys filling the 50-row window).
- Optional `mismatches_only` filter scopes events and jobs to mismatched deployments.
- **`GetDeploymentJobs`** includes `job_id` on each job row for deep links from Queen.

### Queen

- **Accounts:** two-step cluster change — dropdown selects a pending target; **Migrate** commits via `SetAccountCluster`. Per-account success/error messages under the row; when migrations enqueue, copy points to **Admin → Migrations**.
- **Migrations** (`/admin/migrations`): tables for events and jobs, search, mismatch-only toggle, links to deployment detail (events tab + job highlight) and River UI (`?job=`). Job state and River `errors` are shown as-is (no derived “suspicious” flag).
- **Deployment detail:** URL query params for tab and highlighted job; job rows link to River UI by `job_id`.
- API errors show the raw message only (no rollout/branch hints).

### Proto / Queen proxy

- `admin.proto`: `ListClusterMigrations`, `SetAccountClusterResponse.migrations_enqueued` / `deployment_ids`, `DeploymentJob.job_id`, migration event/job message types.
- Queen `admin_handlers` + web client wire the new RPC and response fields.

## Migration

No end-user action required. Ops should expect a brief redeploy window per agent when moving an account between clusters. Knowledge stores remain primary-cluster-only (unchanged).

After preview deploy, wait for **Keel** to roll `astro-server` before verifying new admin gRPC (CI push to ECR does not mean pods have restarted yet).
