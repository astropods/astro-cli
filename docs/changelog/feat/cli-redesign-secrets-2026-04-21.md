## Summary

Introduces `ast secrets` — a new CLI command group for managing account-scoped secrets and plain text variables stored in the Astro vault. Secrets are encrypted at rest; plain text variables are stored as-is. Both types are scoped to the active account and use org-scoped tokens automatically for organization accounts.

## Design

Six subcommands are added under `ast secrets`:

**list** — table output with `Updated`, `Name`, and `Type` columns. `--values` switches the `Type` column to `Value`, showing plain text values and masking secrets as `******`. `--json` emits the raw variable array for scripting.

**get \<name\>** — detail view showing name, description, created, updated, and value (`******` for secrets). `--json` emits the metadata object.

**create \<name\>** — prompts for a value; uses password masking for secrets and visible input when `--plain` is set. `--value <v>` skips the interactive prompt. Outputs `Created secret` or `Created variable` based on the type.

**update \<name\>** — fetches the variable's current type before prompting, so plain text variables get visible input and a `(plain text)` label. `--value <v>` skips the prompt.

**delete \<name\>** — removes the variable from the account vault.

**import \<file\>** — imports `KEY=value` pairs from a `.env` file (parsed by `godotenv`). Variables are stored as secrets by default; `--plain <KEY1,KEY2>` marks specific keys as plain text. Blank values are silently skipped. Existing variables are skipped unless `--overwrite` is set. Sends a single batch request.

Shared HTTP utilities `apiCall` and `apiPath` are introduced in `cmd/api.go`. All secrets commands use these instead of inline `http.NewRequestWithContext` blocks. Account and token resolution goes through `getCurrentAccountToken` (returns an `AccountToken` struct with both account name and scoped token), eliminating repeated boilerplate across handlers.

The server gains a `GET /api/v1/accounts/:account/variables/:varName` endpoint returning variable metadata (value included for plain text, omitted for secrets).

## Migration

No action required.
