# Queen authorization resource inventory

## Summary

Queen now has a top-level **Resources** area for inspecting WorkOS-backed product resources. It gives the private-by-default rollout one place to verify registration, account ownership, assignments, and synchronization before any reset or backfill runs.

Access inspection now lives with the authorization resource instead of the operational deployment view. Deployment resource details reuse Queen's existing member and permission panel and link separately to the deployment runtime page.

## Design

The server reads WorkOS, omits the organization root, and enriches deployments with live Astro account and synchronization data. WorkOS resources and assignments use a shared 30-second cache so concurrent Queen requests do not repeat the same fan-out; database enrichment remains uncached. Per-resource and per-group assignment calls share a bounded concurrency limit. Queen uses manual refresh instead of background polling.

The Resources table shows resource type and name, Astro and WorkOS IDs, account, direct admins, assignment count, creation time, sync state, and the latest error. A WorkOS deployment without a matching Astro deployment is reported as `workos_only`, even when its parent account resolves.

The inventory is read-only. Destructive reset, maintenance, and River reconciliation belong in the stacked follow-up PR.

## Migration

No schema or operator action is required. The inventory is available when WorkOS FGA is configured.
