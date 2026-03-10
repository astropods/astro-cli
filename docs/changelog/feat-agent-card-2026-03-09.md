# Agent Card Spec

## Summary

Introduces the agent card specification — a Markdown file with YAML frontmatter (`AGENT.md`) that serves as the public-facing documentation for an agent. Analogous to HuggingFace's model card, it is purely descriptive and does not drive any functional behavior (deployment, visibility remain in `astropods.yml`).

## Design

The agent card has two parts: structured YAML frontmatter for discovery metadata, and a free-form Markdown body for documentation.

**Frontmatter fields** (all optional):
- `authors` — list of `{name, account?}` entries linking contributors to platform profiles.
- `capabilities` — verb phrases describing what the agent can do.
- `integrations` — third-party services the agent connects to (Slack, GitHub, Jira, etc.).

**Integration matching** uses a known-integrations registry (`packages/astro-spec/agent_card_integrations.json`) with 20 initial entries. Matching is case-insensitive against id and display name. Unknown strings are accepted and displayed with a generic icon.

The file reuses the existing `agent_versions.readme` storage — no schema migration needed. The client parses frontmatter on read to render structured UI (author links, capability badges, brand icons) alongside the Markdown body.

## Migration

No migration required. This is a new spec with no breaking changes. Existing agents without an `AGENT.md` continue to work as before.
