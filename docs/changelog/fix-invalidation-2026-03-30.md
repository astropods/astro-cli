---
title: Fix deployment query invalidation during pause and deploy transitions
date: 2026-03-30
---

## Summary

Pause and deploy state transitions were polling the wrong TanStack Query cache key, causing the detail page to never reflect the updated deployment status (e.g. pause button staying disabled indefinitely in non-local environments where transitions take longer).

## Design

The detail page (`DeployedAgentDetail`) feeds the `deployment` prop from `useDeployment`, which subscribes to `deploymentKeys.detail(id)`. Two polling `useEffect` intervals — one in `ActiveDetailView` (pause flow) and one in `DeploymentsTab` (deploy flow) — were invalidating `deploymentKeys.all(account)` instead. Since `deploymentKeys.all` is a different cache key, those invalidations had no effect on the detail query, so the UI never updated.

Additionally, `deploymentKeys.all(account)` is a prefix of `deploymentKeys.history` and `deploymentKeys.spec`, so those queries were being refetched as collateral on every interval tick — unintentionally polling endpoints that don't need continuous updates during a transition.

Both intervals now target `deploymentKeys.detail(id)` directly. The `DeploymentsTab` interval also explicitly invalidates `deploymentKeys.history` since history records can change on redeploy.

## Migration

No action required.
