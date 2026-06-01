## Summary

Agent subcommands no longer accept a positional deployment target. Targeting is explicit via `--name` or `--id` (mutually exclusive, one required). Removes argv-joining for “unquoted” multi-word names; users quote display names in the shell instead.

## Design

**Flag-only resolution.** `resolveAgentTarget` reads `--id` (deployment detail API) or `--name` (list lookup by display name or blueprint name only). `cobra.NoArgs` rejects stray positionals.

**No duplicate ID paths.** Positional IDs, ID auto-detect, and ID matching on `--name` are removed; deployment IDs use `--id` only.

**Docs and errors.** CLI command tree and `errAgentTargetRequired` describe `--name` / `--id`. Multi-word example uses quoted `--name`.

## Migration

Replace positional targets:

- `ast agent get my-agent` → `ast agent get --name my-agent`
- `ast agent pause ze5-r2l-m16` → `ast agent pause --id ze5-r2l-m16`
- `ast agent get Pirate Parrot EU!` → `ast agent get --name 'Pirate Parrot EU!'`

`--confirm` on delete still accepts display name or deployment ID.

Bump `apps/astro-cli/VERSION` to **0.13.4**.
