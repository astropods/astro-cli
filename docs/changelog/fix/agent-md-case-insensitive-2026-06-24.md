## Summary

Agent README lookups only matched the exact filename `AGENT.md`. On case-sensitive sources — Linux filesystems for the CLI and the GitHub contents API for the server — a repo that stored the file as `agent.md`, `Agent.md`, or any other casing was silently treated as having no README. The agent card (description, tags, body) was then dropped without warning.

## Design

Lookups now match the canonical name `AGENT.md` case-insensitively at each of the four read sites, while creation/scaffolding still writes the uppercase canonical name.

- **CLI (`package cmd`)**: a shared `findAgentReadme(dir)` helper scans the spec's working directory with `os.ReadDir` and returns the first non-directory entry matching `AGENT.md` via `strings.EqualFold`. Used by both the push pipeline's `LoadReadme` and the deprecation-warning check.
- **Server (`githubbuild`)**: a new `FetchAgentReadme(...)` lists the agent's directory through the GitHub contents API, finds the entry whose name matches case-insensitively (filtering to `type == "file"`), then fetches that exact name via the existing `FetchFileContent`. Used by the build pipeline and the draft-card handler.

The server path now makes up to two GitHub API calls (list, then fetch) instead of one, but only when a README exists; the draft-card handler remains Redis-cached, so the extra call is bounded.

## Migration

No action required. Existing `AGENT.md` files keep working; repos using other casings are now picked up automatically.
