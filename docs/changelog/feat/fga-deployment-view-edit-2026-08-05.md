# WorkOS FGA client

## Summary

Add the minimal WorkOS FGA client needed by downstream deployment lifecycle and enforcement work. The client consumes the `deployment:view` and `deployment:edit` permission contracts defined by its PR2 base without changing authorization behavior.

## Design

Astro exposes only the five FGA capabilities needed by the rollout: register and delete resources, assign and remove roles, and check a permission. The implementation delegates request construction, retries, and vendor models to the official WorkOS Authorization SDK. A strict programmable fake gives downstream lifecycle and enforcement tests a stable seam without reproducing the SDK.

The first role slugs are `deployment-viewer` and `deployment-editor`. Roles can be assigned to an individual organization membership or to a group. Groups are collections of memberships, while roles are the permission bundles assigned to those subjects. Assignment and removal support both subject types from the beginning; a live check remains membership-based because WorkOS includes group-derived roles in that result. Group creation and membership management are intentionally deferred.

The existing v6 SDK remains in place for AuthKit integrations, while the FGA implementation uses the current v10 Authorization service. The new client is not wired into server startup or request handling in this change, so no WorkOS calls occur at runtime yet.

## Migration

No user or deployment migration is required. Configure the deployment resource type, permissions, and viewer/editor roles in the preview WorkOS environment before PR4 introduces live resource writes.
