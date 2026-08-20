# Deployment access APIs

## Summary

Deployment administrators can now inspect direct and group-derived access, request Viewer, Builder, or Admin for an organization member, and revoke that member's direct built-in deployment role through Astro's API. This PR adds no customer frontend, group lifecycle, or historical backfill.

## Design

The deployment access routes are cataloged as `deployment:manage_access`, so the same live WorkOS enforcement used by other deployment controls protects every operation. The service separately requires both the global enforcement switch and the organization's Fine-grained access experiment. Disabling either control makes access management unavailable without mutating existing assignments.

Requests use Astro user IDs. Astro validates the target organization membership, records the newest desired role, returns `202 Accepted` with `pending`, `retrying`, or `synced`, and nudges the background reconciler. A failed enqueue does not lose the request because the one-minute sweep repairs durable pending work. Access listings combine effective WorkOS assignments with desired-role status and the latest retry error, making partial operations visible without claiming they already changed authorization.

Changed desired state writes one grant/revoke audit event; idempotent requests do not create duplicates. Cross-tenant and unavailable resources remain concealed as not found, and an organization member whose WorkOS identity is not ready returns an actionable conflict.

## Migration

PR8.3A's access-intent table must be deployed first. No backfill is required. WorkOS environments that enable fine-grained access must contain `deployment-viewer`, `deployment-builder`, and `deployment-admin` with the documented permission bundles.
