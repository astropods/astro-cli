## Summary

`ast agent` subcommands that take a deployment target required a single quoted argument and only matched display names in the list response. Names with spaces or shell-special characters (e.g. `Pirate Parrot EU!`) were awkward in zsh, and deployment IDs from `agent list` could not be passed positionally.

## Design

**Shared target resolution.** Agent subcommands (`get`, `pause`, `resume`, `delete`, `history`, `restart`, `logs`, `redeploy`) route through `resolveAgentTarget`, which returns a deployment record before any action runs.

Resolution order:

1. `--id` — fetch deployment detail by ID (validates the deployment exists).
2. Positional args joined with spaces — if the result matches the `xxx-xxx-xxx` ID pattern, resolve via deployment detail; otherwise list lookup.
3. List lookup — match display name, blueprint name, or ID.

**Multi-word display names.** Positional args are joined rather than limited to one token, so `ast agent get Pirate Parrot EU!` works without quoting.

**Consistent `--id`.** The flag is registered on all target-taking subcommands, not only pause/resume/restart/logs.

**User-facing labels.** Commands print the display name (falling back to blueprint name) in status output and prompts. `delete --confirm` accepts either the display name or deployment ID.

**History API path.** After resolution, `agent history` uses the blueprint name from the resolved deployment for the history endpoint, not the raw user input (which may be a display name or ID).

## Migration

No changes required. Existing single-token names and `--id` usage continue to work. Multi-word names and positional IDs are optional shortcuts.

Bump `apps/astro-cli/VERSION` to **0.13.3** and release via `Release CLI (Prod)` so `ast upgrade` picks up the new target resolution behavior.
