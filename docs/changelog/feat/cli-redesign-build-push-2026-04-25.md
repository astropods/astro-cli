## Summary

Completes the `ast blueprint` CLI redesign by making `build` and `push` proper subcommands of `bp`, adding a `validate` command, converting push and build to pure functions with explicit parameters, and establishing a clean validation-before-auth ordering guarantee.

## Design

### Command surface

`blueprint push <name>` and `blueprint build <name>` now require the agent name as a positional argument. The name overrides whatever is in the spec — if it differs from the spec name, the CLI warns before proceeding. Top-level aliases `ast push <name>`, `ast build <name>`, and `ast validate` are registered at the root so existing workflows continue to work.

`blueprint validate` (alias `ast validate`) validates `astropods.yml` against the JSON schema and semantic rules without authenticating or touching the registry. Uses the same `runValidate(specPath string)` logic as the inline pre-checks, so output format and error messages are identical regardless of how validation is triggered. `-f` selects an alternate spec file.

`blueprint set` — the existing command for updating blueprint settings — is what the command tree spec called `blueprint visibility`. It exposes `--visibility public|private`.

### Push as a pure function

`runPush(ctx, AccountToken, pushConfig)` no longer owns auth or validation. Auth is resolved in the cobra handler via `cmdAuth(cmd)` which returns an `AccountToken{Account, Token, ExpiresAt}`. The handler passes it directly to `runPush`. Package-level flag variables (`agentNameOverride`, `skipBuild`, etc.) are gone; all config flows through `pushConfig`.

The same pattern applies to `runBuild(ctx, specPath, agentName, tag, platforms, ...)` — it is a plain function with no cobra dependency. The cobra handler resolves the spec path and validates before calling it.

Both functions carry a contract comment: callers must validate the spec before invoking.

### Validation ordering

The cobra handlers for `push` and `build` both call `validateSpecFile` before doing anything else — before auth for push, before the Docker build for build. This ensures a bad spec produces an immediate, actionable error without triggering authentication or container machinery. `runPush` and `runBuild` no longer call `validateSpecFile` themselves; they call `spec.ParseSpec` directly for the parsed spec data they need.

### Scoped `-f` flag

The global persistent `-f/--file` flag is removed from the root command. The flag is now registered per-command on `blueprint push`, `blueprint build`, `blueprint validate`, and their top-level aliases. Commands that don't take a spec file (e.g. `blueprint list`, `blueprint get`) no longer advertise it.

### Auth helper consolidation

`cmdAuth(cmd)` is the single call-site for auth in all cobra handlers: it calls `getCurrentAccountToken`, reads the verbose flag, and returns `(AccountToken, bool, error)`. Direct use of `getUserNamespace`, `auth.NewTokenManager`, or `GetOrgScopedAccessToken` in handlers is gone.

## Migration

No action required. Top-level `ast push <name>` and `ast build <name>` continue to work as before; `ast validate` is new. Users who passed `--skip-build`, `--skip-push`, `--skip-register`, `--no-auth`, `--server`, `--registry`, or `--platform` to `ast push` will get an unknown-flag error — those flags are removed.
