# Add `repository` field to agent card

## Summary

Agents can now link to their source-code repository via a `repository` field in the AGENT.md frontmatter. Supports shorthand strings (`github:user/repo`, `gitlab:user/repo`, `bitbucket:user/repo`, bare `user/repo`) and an object form with `type`, `url`, and `directory` for monorepos.

## Design

The field is parsed in `packages/astro-spec` with a custom YAML unmarshaler that accepts both string and object forms. Shorthand strings are resolved to full URLs at parse time. The parsed `AgentCardRepo` struct (with `type`, `url`, `directory`) is serialized into `agent_card_json` at registration and served to the client.

On the client, a new `SidebarRepository` component renders the repository link below the authors section on the blueprint detail page. Provider detection is based on the URL hostname — known providers (GitHub, GitLab, Bitbucket) display their brand icon from the integration icon CDN; unrecognized providers show just the text link.

GitLab and Bitbucket were added to the known integrations registry to support icon resolution.

## Migration

No migration required. The field is optional — existing agents without it are unaffected.
