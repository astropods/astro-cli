# Deployment access provisioning

## Summary

Fresh organization deployments now remain visible to their creator while Astro finishes registering the deployment and its `deployment-admin` assignment in WorkOS. The deployment stays in a clear "Setting up access" state instead of disappearing or opening into a temporary authorization error.

The live WorkOS discovery behind this state is also bounded and observable: a slow, canceled, or misconfigured organization cannot silently hang every deployment list.

## Design

- `/me/deployments` reports `access_ready` from the deployment FGA lifecycle ledger; list shapes that do not compute readiness omit the field.
- The deploy receipt supplies a temporary card only until the real deployment row appears, and only on the first page when it matches the active account and search filters.
- The card and deployment reveal overlay keep navigation fail closed while access is pending. A live status replaces the misleading disabled action, and the reveal always offers a clear return to the agents list.
- The client polls every two seconds for the first ten seconds, then every ten seconds for up to two minutes. A stalled setup stops automatic polling and offers an explicit retry or exit while keeping navigation fail closed.
- Pending responses bypass the remote list cache during the two-minute provisioning window so the UI can observe convergence. A stalled assignment then returns to the normal 30-second cache without changing its fail-closed `access_ready=false` state.
- Discovery resolves and caches WorkOS's immutable internal organization-root ID before listing readable child resources. This avoids treating the WorkOS organization ID as an authorization-resource external ID.
- The complete discovery fan-out has a two-second request budget. Ordinary failures stay fail closed for the affected organization and emit an account-scoped warning; deadline exhaustion uses the existing `503` path.
- Concurrent cache misses share one bounded WorkOS lookup that is independent of the initiating HTTP request, so a canceled browser request cannot fail sibling requests waiting on the same result. Cold organization-root resolution is also coalesced per organization.

Authorization remains fail closed: this state does not grant access and ends only after the WorkOS resource and creator role assignment are synchronized.

## Migration

No user action is required.
