# Fix stale paused state after resuming an agent

## Summary

Resuming a paused agent left the UI in a contradictory state: the status toggle showed "Pausing", the deployment history showed "Deploying", and the pod tiles still showed "Paused". A page refresh cleared it. This fixes the resume flow so the UI reconciles on its own.

## Design

Resume (`useWakeUpDeployment`) optimistically updated the deployment list and the status query, but never invalidated the deployment record. The record therefore stayed at its pre-resume `stopped` value until a manual refresh. The status toggle derives "active vs paused" from that record (`isPausedState`) while deriving "transitioning" from the status query, so a `stopped` record next to a `deploying` status resolves to the nonsensical "Pausing" label; the pod tiles read the same record and stayed "Paused".

The fix mirrors pause and restart: on success, resume now invalidates the deployment record (and its runtime and status children) via the shared `invalidateDeployment` helper, so the record refetches the transitional status the server already set on wakeup and every reader reconciles without a refresh.

## Migration

None.
