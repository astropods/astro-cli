# Summary

Re-pushing an already-registered blueprint was wrongly blocked once an account
hit its blueprint-count limit, even though the push creates no new blueprint. The
resulting 402 also spoke of "agents" (the internal feature key) rather than
blueprints, and the CLI rendered it as an escaped JSON body plus a raw Go
`map[...]` dump.

# Design

**Gate the blueprint-count limit only on new blueprints.** The register (push)
route was guarded on both `agents` (non-archived blueprint count) and
`agent_builds` (builds this billing period). Every push is a build, so
`agent_builds` still applies to all pushes; but a re-push does not add to the
blueprint count and must not be gated by it.

A dedicated `DBChecker.WrapRegister` replaces the generic `Wrap(..., "agents",
"agent_builds")` on the route. It always checks `agent_builds`, and adds the
`agents` check only when the push would create a new non-archived blueprint. The
register upsert un-archives on name conflict, so "new" means the name is absent
or currently archived; `blueprintExists` tests for a live row
(`archived_at IS NULL`). The existence check fails open on a DB error (the build
limit still applies).

**Say blueprints, not agents, with counts.** The blueprint-count resource now
renders as "Blueprints" in 402 bodies, mentions archiving as a remedy, and the
limit-reached detail includes the usage/limit counts (e.g. "Blueprints limit
reached (36 of 5 used): ...").

**Rename the quota key `agents` -> `blueprints`.** The internal quota resource
identifier is renamed to match the user-facing noun (the quota system is not yet
enforced, so this is safe). This touches the quota package, the config default
key (`QUOTA_DEFAULTS`), the quota-increase feature-key allowlist, the usage
`meters` map key, and the client label/meter references. The DB *table* stays
`agents` (the count query is unchanged); only the resource identifier moves.

**Readable CLI error.** `ast push` now prints the server's `details` message on a
non-201, instead of logging the raw escaped body and returning a Go-map dump. The
raw body is logged only under `--verbose`.

# Migration

The blueprint-count quota key changes from `agents` to `blueprints`. A migration
(`sql/river/002_rename_blueprints_quota_key.sql`) updates any existing
`account_limits` override rows automatically on deploy, so overrides keep applying
once enforcement is turned on; no manual step is required.

A `QUOTA_DEFAULTS` env override using `agents=...` must switch to `blueprints=...`.
No API-response shape or limit-value changes otherwise. Closes #1696 and #1697.
