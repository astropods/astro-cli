# Agent Card Implementation

## Summary

Implements the agent card system end-to-end: server-side parsing and storage of AGENT.md frontmatter, a pre-parsed `agent_card_json` column for efficient reads, integration resolution against a known registry, and a redesigned client detail page that renders structured metadata (authors, capabilities, integrations) alongside the Markdown body.

## Design

**Server & storage** — When an agent is registered, the server parses the AGENT.md frontmatter and merges spec-derived integrations (from `astropods.yml` provider fields) into a single resolved list. The result is serialized and stored in `agent_versions.agent_card_json` (JSONB) so reads don't re-parse on every request. Legacy agents without an AGENT.md get a synthesized card from deprecated `meta.description` and `meta.tags` fields.

**Integration resolution** — A known-integrations registry (`agent_card_integrations.json`) maps user-supplied strings to canonical IDs via case-insensitive matching against id, display name, and aliases. Resolved integrations carry both an ID (for icon lookup) and a display name. Unknown strings pass through with a generated ID.

**Client detail page** — The agent detail page renders the structured card: author sidebar with account links, capability badges, and branded integration icons (served from a static assets bucket with light/dark variants). The Markdown body is rendered below with image unwrapping for cleaner layouts.

**Schema** — `agent_card_json` uses the `jsonb` type rather than `text`, giving write-time JSON validation, more compact binary storage, and the option to add GIN indexes later for querying by integration or tag without a migration.

## Migration

No user-facing migration required. Existing agents without AGENT.md continue to display via the legacy fallback path. The JSONB column defaults to `{}` — existing rows with empty strings need a one-time `UPDATE ... SET agent_card_json = '{}'` before the Atlas-generated `ALTER COLUMN` runs.
