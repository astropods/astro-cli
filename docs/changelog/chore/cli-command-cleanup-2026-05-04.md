## Summary

Removes the last dead-code path for account resolution (`getUserNamespace`), migrates knowledge commands to the shared `apiCall`/`apiStream` utilities, renames the `configure --out` flag, and wires up the previously inert `--no-pull` flag in `project start`.

## Design

**`getUserNamespace` removed** from `cmd/push.go` along with its six unit tests. The function was originally the shared mechanism for reading the active account from stored credentials. As each command was migrated to `cmdAuth`, it became unreachable from any handler; the `knowledge` commands were the last caller before they were updated in the parent branch.

**Knowledge commands migrated to `apiCall`/`apiStream`**: the hand-rolled `knowledgeRequest` helper is removed. All non-streaming handlers now use `apiCall` (which handles marshalling, auth headers, and status-code-aware error returns); historical log fetches use `apiStream`. The SSE tail (`--tail`) retains a manual HTTP request because it needs `Accept: text/event-stream`. The per-command `--output`/`-o` string flag is replaced with `--json bool` to match the convention used by blueprint and agent commands.

**`configure --out` renamed to `--output`/`-o`** to match the shorthand convention used by `knowledge list`, `knowledge status`, and `knowledge credentials`. The docs-public CLI reference is updated to match.

**`--no-pull` wired in `project start`**: the flag was registered but its value was never consumed. It now sets each service's `PullPolicy` to `"never"` on the project passed to `compose Up`, preventing image pulls when only locally built images should be used. `--local` applies the same policy since it implies no-pull.

## Migration

- Users who pass `--out env` or `--out json` to `ast configure` / `ast project configure` must update to `--output env` / `--output json` (or `-o env` / `-o json`).
- Users who used `--output json` on `knowledge list`, `knowledge status`, or `knowledge credentials` must switch to `--json`.
