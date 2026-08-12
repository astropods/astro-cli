# Deployment authorization catalog

## Summary

Define the deployment permission contract before enforcement begins. PR6 maps deployment routes to five practical capabilities without creating one permission per endpoint or UI button.

This remains shadow-only: existing membership authorization still decides every response, and no historical deployment is backfilled.

## Design

The WorkOS-backed permission slugs are:

- `deployment:read` — detail, configuration metadata, logs, traces, monitoring, files, and the caller's own alert subscription.
- `deployment:edit` — metadata, avatar, and files.
- `deployment:operate` — redeploy, rollback, restart, stop, resume, cancel, and ingestion.
- `deployment:delete` — permanent removal.
- `deployment:manage_access` — grant and revoke access; APIs land later.

Permissions are flat; roles bundle them. `deployment-reader` contains read, while `deployment-editor` contains read, edit, operate, and delete. Owners/admins inherit all five and member remains empty.

Deployment-ID routes register their handler and authorization classification together. Startup fails if a live route lacks one. User mutations and body-addressed redeploy/delete attempts use PR5's bounded shadow checks; frequently fetched reads are deferred to avoid a WorkOS call per refresh. Chat and messaging remain a separate data plane.

Evaluation URLs currently contain a deployment ID, but their resource ownership model is unresolved. Dataset, prediction, review, and judgment routes are explicitly model-deferred: they keep legacy membership behavior and send no deployment permission to WorkOS.

### Review and Preview proof

Confirm the five constants match WorkOS, deployment routes use read/edit/operate/delete as described, and evaluation routes are model-deferred. In Preview, exercise reads, edits, lifecycle operations, and undeploy; logs should show the matching slug without changing HTTP behavior. Evaluation requests should produce no deployment FGA check.

PR7 adds the effective-capabilities response and staged enforcement. PR8 adds access, groups, and discovery APIs; PR9 completes API-level acceptance and cleanup.

## Migration

No database, backfill, or client migration is required. Configure the five permission slugs and role bundles in WorkOS Preview before deployment. Production remains non-enforcing until the later rollout.
