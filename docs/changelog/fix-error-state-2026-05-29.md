## Summary

Stopped deployments paused via the queen admin UI showed an "Error" badge in the astro-client agents page instead of "Inactive/Stopped". This was a display bug — the DB status was correctly `stopped`, but the agents list API was misclassifying it.

## Design

Two bugs combined to cause this:

**Bug 1 — `agentDeploymentFromDB` over-broad error condition.** The DB-only status-building path used by `ListDeployments` (the agents page list endpoint) had a condition that overrode the mapped status to `"error"` whenever `error_message` was non-empty, regardless of the actual `status` column value:

```go
// Before
if dep.Status == StatusFailed || (dep.ErrorMessage != nil && *dep.ErrorMessage != "") {
    ad.Status = "error"
}

// After
if dep.Status == StatusFailed {
    ad.Status = "error"
}
```

**Bug 2 — `error_message` was used as a general message field.** `UpdateStatus` wrote the same string to both `deployments.error_message` (the deployment row) and `deployment_events.message` (the event log). Admin and reconciler transitions stored informational context like `"Admin stop requested"` in `error_message` even though they weren't errors. Combined with Bug 1, this caused any deployment touched by an admin action to show an error badge.

The fix separates the two concerns. `UpdateStatus` now takes distinct `errorMsg` and `eventMsg` parameters:

- `errorMsg` → `deployments.error_message` (set only for `StatusFailed`)
- `eventMsg` → `deployment_events.message` (human-readable context for any transition; falls back to `errorMsg` when empty so error callers don't need to repeat themselves)

Informational transitions (admin stop/wakeup/re-apply, KEDA scale-down, OIDC drift reapply, rollback) now pass `""` for `errorMsg` and their message as `eventMsg`. The messages remain visible in queen's deployment detail event timeline.

Existing rows in the DB with `status = stopped` and a non-empty `error_message` are corrected by Bug 1's change alone — they will correctly display as stopped on next page load.

## Migration

No action required. Existing affected deployments will show the correct stopped status on next page load.
