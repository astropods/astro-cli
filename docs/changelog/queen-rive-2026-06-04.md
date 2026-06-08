## Summary

Adds a first-class Jobs dashboard to the Queen admin UI, replacing the previous River UI iframe embed. The dashboard shows live job state counts, queue status with pause/resume, job history with state/kind/queue filtering, running jobs with cancel, a registered worker registry with one-click trigger, and inline error inspection — all without requiring River UI to be running.

## Design

### Dynamic worker registry (`riverqueue/workers.go`)

River job kinds are registered beside each job args type, independent of worker construction. This gives API-only processes the same job catalog as worker processes, so Queen can list and trigger jobs in production without relying on worker pods running in the same process. Worker registration still checks the catalog and logs drift, while tests enforce that every args type with a `Kind()` method is registered.

`RegisteredJobKinds()` returns a sorted catalog with a zero-value args JSON schema for trigger pre-fill. `Queue.TriggerJob(ctx, kind, argsJSON)` dispatches through typed trigger functions so manually triggered jobs use the same River args types as normal enqueues.

### Admin gRPC RPCs

The AdminService contract now declares the complete job dashboard API used by Queen:

- `ListJobKinds` / `TriggerJob` — worker registry and manual trigger
- `GetJobStates` — `GROUP BY state` on `river.river_job`
- `ListAdminQueues` — `river.river_queue` joined with per-queue available/running counts
- `ListJobs(state, kinds[], queue, limit, before_id, anchor_id)` — filtered cursor pagination over `river.river_job`
- `GetJob(id)` — single row including full `errors` JSONB
- `CancelJobs(ids[])` — `river.Client.JobCancel` per ID
- `RetryJobs(ids[])` — direct SQL move back to `available` without resetting attempt count
- `PauseQueue(name)` / `ResumeQueue(name)` — `river.Client.QueuePause` / `QueueResume`

Read RPCs return gRPC errors when River tables cannot be queried, so Queen shows an explicit failure instead of empty counts or empty tables. Retry responses count only jobs that were actually updated.

### Queen HTTP layer (`riverui_handlers.go`)

River UI proxy routes (`/riverui/`, start/stop/status) removed. Replaced with ten direct job routes under `/api/admin/jobs/*` and `/api/admin/queues/*`.

### Jobs page (`/admin/jobs`)

Four-tab layout: **Overview** (state counts + queue table), **History** (server-paginated table, expandable rows show args + errors), **Running** (auto-refreshes every 5 s), **Workers** (registry table with last-seen queue/time and Trigger button). History filters run on the server before pagination, and the page uses cursor metadata (`next_before_id`, `has_more`) instead of loading a fixed-size local history window.

Deep links use `/admin/jobs?job=<id>`. Queen requests an anchored history page around that job, then scrolls to and expands it, so old jobs linked from deployment detail or migrations still render even when they are outside the newest page. Query and mutation failures render inline error notices instead of silent empty states.

### Removed

- `apps/astro-queen/web/src/pages/river-ui.tsx` — iframe embed page deleted
- `/admin/river-ui` route removed from router
- All River UI-dependent hooks (`useRiverUIStatus`, `useStartRiverUI`, `useRiverJobs`, etc.) removed from `admin.ts`; replaced with `useJobStates`, `useAdminQueues`, `useAdminJobs`, `useAdminJob`
- Auto-start logic and `enabled: status?.running` guards removed — all queries run unconditionally

Deep links that previously pointed to `/admin/river-ui?job=<id>` (in deployment detail and migrations pages) updated to `/admin/jobs?job=<id>`.

## Migration

No user-facing action required. River UI continues running inside astro-server for job processing; it is no longer surfaced as a UI dependency in Queen.
