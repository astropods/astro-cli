# CLI Command Tree Redesign

## Summary

The current CLI has a flat, mixed command structure with no consistent grouping or mental model. This proposal introduces a noun-verb command tree that organizes commands by resource, adds an org-scoping layer, and aligns the CLI surface to the full server API.

## Design

Commands follow a strict `ast <noun> <verb>` pattern. Resources are:

- **`org`** — `list`, `switch`: sets the active org that scopes all other commands.
- **`blueprint`** — `list`, `create`, `get`, `push`, `archive`, `visibility`: manages registered agent definitions on the platform. `push` is the publish operation (build + registry push + spec registration).
- **`agent`** — `list`, `deploy`, `get`, `delete`, `stop`, `start`, `restart`, `rollback`, `history`, `validate`, `trigger`, `logs`: manages running deployments. Covers the full deployment lifecycle including `validate` (dry-run), `trigger` for manual ingestion runs, and `logs` (with `--follow` for streaming).
- **`secret`** — `list`, `create`, `update`, `delete`: manages account vault variables. Values are write-only after creation.
- **`project`** — `init`, `configure`, `dev`, `build`, `validate`, `add`, `explain`: purely local operations. `init` replaces `ast create`; `build` is now an explicit step separable from push.

Key distinction: `project` commands are local-only; `blueprint` commands are server-only. They compose naturally — scaffold locally with `project init`, register on the server with `blueprint create`, then ship with `blueprint push`.

A global `--org` flag overrides the active org per command.

## Migration

Existing commands are renamed into the new tree (see spec for full mapping). No behavioral changes in this proposal — restructuring only.
