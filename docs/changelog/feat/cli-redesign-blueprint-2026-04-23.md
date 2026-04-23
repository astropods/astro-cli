## Summary

Introduces `ast blueprint` — a new CLI command group for managing agent blueprints registered on the platform. Blueprints are versioned agent definitions; this command group covers the full lifecycle from registration through archival.

## Design

Six subcommands are added under `ast blueprint` (alias `bp`):

**list** — table output with `Published`, `Build`, `Visibility`, `Deploys`, and `Name` columns. `Published` shows the latest `published_at` across versions, falling back to `pending` for blueprints with no versions yet. `--json` emits the full blueprint array for scripting.

**get \<name\>** — detail view showing name, account, visibility, deploy count, archived date, and version history. When a blueprint has no versions yet, a next-steps box is shown in place of the version list to guide the user toward their first push. `--json` emits the full metadata object.

**create \<name\>** — registers a new blueprint. Defaults to `private` visibility; `--visibility public` makes it publicly discoverable. `--visibility` is validated strictly — invalid values are rejected (no silent coercion). Friendly error on conflict (409). On success, displays a next-steps box pointing to `ast blueprint push`.

**update \<name\>** — updates blueprint settings. Currently supports `--visibility public|private`, validated via the same `validateVisibility` helper as `create`. Returns an error without calling the server if no flags are set.

**archive \<name\>** — soft-deletes the blueprint. Friendly error on not-found (404).

**push** — delegates to `runPush` with `--build` to optionally build the image first and `--visibility` to set visibility at push time. Intentionally exposes a narrower flag set than `ast push` directly: `--platform`, `--server`, `--registry`, `--skip-push`, `--skip-register`, and `--no-auth` are not surfaced. The target platform is hard-coded to `linux/amd64`. The omitted flags no longer make sense once account management and scoped tokens are in place. `ast push` will be removed in a future PR; `ast blueprint push` is the intended replacement.

All handlers follow the same conventions established for `ast secrets`: `apiCall` returns `(int, error)` so status codes drive friendly messages without string matching, all output goes through `cmd.OutOrStdout()` (including the tabwriter), and `--json` flags are registered per-command via `Bool()` with no shared package-level state.

## Migration

No action required.
