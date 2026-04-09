# Cancel Superseded GitHub Builds on Rapid Pushes

## Summary

When multiple pushes arrive in rapid succession for the same connected repo, older builds were allowed to run to completion even though their output would be stale. The last build to *finish* (not the last to *start*) determined what got deployed, which could mean deploying an older commit. This change implements "last push wins" semantics across the full build lifecycle.

## Design

Cancellation is wired end-to-end through three layers:

**DB layer (`githubconnection/store.go`)**: Three new methods handle the state transitions.
- `CancelOlderBuilds(connectionID, keepID)` — bulk-marks all non-terminal builds for a connection (except the new one) as `cancelled` with `completed_at`. This ensures the UI immediately reflects that older builds were superseded.
- `CancelBuild(id)` — single-build variant used by the worker when its context is cancelled mid-flight.
- `StartBuildIfPending(id) (bool, error)` — atomically transitions `pending→building` only if the row still has `status = 'pending'`. Returns `false` if a newer push already cancelled it. This prevents a race where a worker picks up a job just after `CancelOlderBuilds` sets it to `cancelled`.

**Queue layer (`riverqueue/client.go`)**: `CancelGitHubBuildsForConnection` queries `river.river_jobs` directly for all active jobs (`available`, `pending`, `running`, `scheduled`) matching the connection ID and calls River's `JobCancel` on each. For running jobs, this sends a context cancellation signal to the worker goroutine. That cancellation propagates into `RunJob`'s poll loop via `pollCtx`, which exits and triggers the existing `defer` that calls `k8s.BatchV1().Jobs.Delete` — killing the BuildKit pod.

**Webhook handler (`handlers/github.go`)**: After `CreateBuild`, the handler calls both `CancelOlderBuilds` (DB) and `CancelGitHubBuildsForConnection` (River/K8s) before enqueuing the new job. Ordering ensures the new job is never present in River when the cancellation query runs.

**Worker (`riverqueue/github_build.go`)**: Two changes handle cancellation in the worker:
1. The initial `UpdateBuildStatus(..., "building")` is replaced with `StartBuildIfPending`. If it returns `false`, the worker returns `river.JobCancel` immediately — no K8s job is ever created.
2. If `RunJob` returns `context.Canceled` (from River's `JobCancel` signal), the worker calls `CancelBuild` to update the DB record and returns `river.JobCancel` to suppress retries.

The `cancelled` status is a new terminal state alongside `registered` and `failed`, surfaced in the UI's build history.

## Migration

No schema migration required — `status` is a free-form text column with no enum constraint.
