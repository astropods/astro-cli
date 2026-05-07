# fix-ingestion

## Summary

Scheduled ingestion was invisible in the deployment view. The `GetDeployment`
endpoint listed Deployments, StatefulSets, Jobs, Ingresses, and Pods, but
never queried CronJobs — a configured schedule trigger only appeared once it
had fired its first child Job. A misconfigured or suspended cron looked
identical to "no ingestion at all."

The fix unifies the deployment view's data shape: ingestion definitions
(both one-shot Jobs and recurring CronJobs) are now first-class Workloads
alongside Deployments and StatefulSets. The separate top-level `Jobs[]`
field on `AgentDeployment` is gone — run history hangs off the parent
Workload that produced it.

## Design

**Workload kinds extended.** `WorkloadDetail.Kind` now takes one of
`Deployment`, `StatefulSet`, `Job`, or `CronJob`. The struct grew a small
set of omitempty fields used only by batch kinds:

```go
type WorkloadDetail struct {
    // ... existing fields ...
    Status      string      // Job: Pending/Running/Succeeded/Failed.
                            // CronJob: Idle/Active/Suspended.
    Schedule    string      // cron expression (CronJob only)
    StartTime   string      // Job: pod start. CronJob: last fire time.
    Completions string      // "succeeded/desired" (Job only)
    Runs        []JobDetail // CronJob children (oldest-first)
}
```

Long-running kinds (Deployment/StatefulSet) leave the new fields empty —
their health is still read from `Containers[].Ready`. Frontends switch on
`Kind` to render appropriately.

**ownerRef routes Jobs to Runs[] or to their own row.** When listing Jobs,
each one is checked for a `batch/v1 CronJob` ownerReference:

- Owned by a CronJob in the same namespace listing → appended as a
  `JobDetail` to that CronJob's `Workload.Runs`.
- Standalone (startup ingestion) → its own `Kind="Job"` Workload with
  `Status`, `StartTime`, `Completions` populated directly.
- Owned by a CronJob that wasn't returned (label-mismatched, pruned) →
  falls through and surfaces as its own Workload so it stays visible.

CronJobs are listed before Jobs so the parent-by-name index is ready when
the Job loop runs.

**Status vocabulary.** A new `cronJobStatus` helper encodes the CronJob
states (`Suspended` when `spec.suspend=true`, `Active` when
`status.active` is non-empty, `Idle` otherwise). Job status keeps the
existing `jobStatus` vocabulary.

## Migration

API consumers should update reads as follows:

- `AgentDeployment.jobs[]` → gone. Iterate `workloads[]` instead and
  branch on `kind`.
- For run history of a scheduled ingestion, read `workload.runs` on the
  `kind="CronJob"` entry.
- For a one-shot ingestion's status, read it directly off the
  `kind="Job"` workload (no nested run).

The astro-client `AgentDeployment` TypeScript type is updated in lockstep.
The astro-queen TUI uses a separate admin `/jobs` endpoint and is
unaffected.
