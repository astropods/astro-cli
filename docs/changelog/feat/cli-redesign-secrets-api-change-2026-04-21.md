## Summary

Introduces `ast secrets` — a new CLI command group for managing account-scoped secrets and plain text variables stored in the Astro vault. Secrets are encrypted at rest (KMS); plain text variables are stored as-is. The server gains a single-variable GET endpoint to support type-aware CLI interactions.

## Design

### Server: GET /variables/:varName

A new `GetAccountVariable` handler mirrors the shape of the existing `ListAccountVariables` response but for a single variable. It reuses `store.Get` (already used by the update handler) and applies the same value-visibility rule as the list: `value` is populated for plain text variables and omitted for secrets. This endpoint allows the CLI to determine a variable's type before prompting, without fetching the entire list.

### CLI: ast secrets

Five subcommands are added under `ast secrets`:

- **list** — table output with `Updated`, `Name`, and `Type` columns. `--values` switches the `Type` column to `Value`, showing plaintext values and masking secrets as `******`. `--json` emits the raw variable array.
- **get \<name\>** — detail view (name, description, created, updated, value or `(secret)`). `--json` emits the metadata object.
- **create \<name\>** — prompts for value; uses password masking for secrets and visible input for `--plain`. `--value` skips the prompt. Outputs `Created secret` or `Created variable` accordingly.
- **update \<name\>** — fetches the variable's type first via GET, then prompts with the appropriate echo mode and labels the prompt `(plain text)` for variables. `--value` skips the prompt. The secret/plain type cannot be changed on update.
- **delete \<name\>** — removes the variable from the account vault.

All subcommands resolve the current account and obtain an org-scoped token when the account is a WorkOS organization.

## Migration

No action required. Existing credentials and configuration are unaffected.
