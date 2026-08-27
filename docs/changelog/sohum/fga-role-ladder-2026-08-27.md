# Fine-grained access role ladder

## Summary

Deployment access now uses the WorkOS role ladder defined by the private-by-default model: Viewer, Writer, Maintainer, and Admin. This removes the obsolete Builder role before resource backfill and access UI work.

## Design

Writer grants `deployment:read` and `deployment:edit`. Maintainer adds `deployment:operate`. Admin adds `deployment:delete` and `deployment:manage_access`. Astro APIs expose those four levels and send their matching external role slugs to WorkOS.

The Preview reset deleted WorkOS assignments but not Astro's durable access-intent ledger. At startup, existing `deployment-builder` intent is moved to `deployment-maintainer`, the closest least-privileged replacement. Reconciliation removes any old direct Builder assignment and applies Maintainer without changing group-derived or custom roles.

## Migration

Apply the WorkOS permissions and roles from `scripts/workos-fga/model.json` before deploying. API callers must replace `builder` with `writer` or `maintainer`; existing durable Builder intent migrates to Maintainer automatically.
