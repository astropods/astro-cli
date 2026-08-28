# Fine-grained access role ladder

## Summary

Deployment access now uses the WorkOS role ladder defined by the private-by-default model: Viewer, Writer, Maintainer, and Admin. This removes the obsolete Builder role before resource backfill and access UI work.

## Design

Writer grants `deployment:read` and `deployment:edit`. Maintainer adds `deployment:operate`. Admin adds `deployment:delete` and `deployment:manage_access`. Astro APIs expose those four levels and send their matching external role slugs to WorkOS.

Builder is removed outright rather than aliased. The role no longer exists in WorkOS, `scripts/workos-fga/model.json` no longer defines it, and no `deployment-builder` rows remain in the `resource_access_fga_sync` ledger in any environment. Nothing can produce the slug either: the only path into `desired_role` is `RoleForAccessLevel`, which reads the catalog in `access_catalog.go`. A compatibility alias would therefore be unreachable code, so the codebase carries no legacy Builder handling.

## Migration

Apply the WorkOS permissions and roles from `scripts/workos-fga/model.json` before deploying. API callers must replace `builder` with `writer` or `maintainer`. No data migration is required; verify with `SELECT count(*) FROM resource_access_fga_sync WHERE desired_role = 'deployment-builder'`, which returns zero.
