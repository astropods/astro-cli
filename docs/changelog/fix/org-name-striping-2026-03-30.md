# Fix: Strip @org/ prefix from agent names during push

## Summary

Agents with org-scoped names (e.g. `@postman/feb19-astro`) in `astropods.yml` caused
"source agent not found" errors during deploy. The CLI stripped the prefix for the
registry URL and agent index key, but the stored spec YAML retained the full scoped name.
When the deploy template was generated from the stored spec, `source.name` contained the
prefix, which didn't match the agent index entry.

## Design

**CLI fix (`transformSpecForRegistry`):** Strips the `@org/` prefix from the spec's `name`
field before uploading, so the stored spec matches the indexed agent name. Uses the
existing `ParseAgentName` utility.

**Server-side guard (register + deploy handlers):** Rejects agent names containing `/` or
starting with `@` with a 400 error, providing a clear message to upgrade the CLI. This
catches any scoped names that slip through from older CLI versions.

## Migration

No migration required. Agents previously pushed with scoped names need to be re-pushed
with CLI v0.7.2+ to fix their stored spec.
