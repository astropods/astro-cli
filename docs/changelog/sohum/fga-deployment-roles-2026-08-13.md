# Deployment role contract

## Summary

Replace the temporary Reader/Editor role model with the deployment roles used by the access-management APIs: Viewer, Builder, and Owner. New deployment creators receive Owner so they can control both their deployment and who may access it.

## Design

Permissions remain the stable authorization contract; roles only bundle them:

- Viewer contains `deployment:read`.
- Builder contains read, edit, operate, and delete, but cannot grant access.
- Owner contains all five deployment permissions, including `deployment:manage_access`.

Astro still checks permissions rather than role names. The global enforcement switch and organization experiment continue to guard every enforced decision; this PR only changes the role assigned during deployment reconciliation.

Previously synchronized deployments are not mutated here. PR9's idempotent backfill will repair historical creator assignments before the access-management UI ships.

## Migration

Configure the three role slugs in each WorkOS environment before deploying. No database migration is required.
