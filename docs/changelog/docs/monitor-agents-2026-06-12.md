# Monitor your agents

## Summary

The `ast docs agent` instrumentation guide (added 2026-06-01) named the right library for each stack but lived inside CLI output. Users browsing the public Fern docs had no equivalent page, so the existence of the official `@astropods/observability-mastra` and `@astropods/adapter-claude-agent-sdk` adapters was discoverable only from inside a deployed project. This adds a public docs surface for monitoring, including per-framework adapter pages.

## Design

### Surfaces touched

- **Public Fern docs** — new "Monitor your agents" section with an overview page and per-framework guides for Mastra and Claude Agent SDK.
- **In-CLI `ast docs agent` guide** (`apps/astro-cli/cmd/docs/agent_instructions.md`) — refreshed Mastra and Claude Agent SDK subsections, added a "Manual setup (Node.js)" subsection that mirrors the public-docs example. Kept LangChain / Python and raw Anthropic / OpenAI subsections unchanged.

### Mastra reality vs. earlier draft

The first draft of `monitor-mastra.mdx` referred to a `@astropods/observability-mastra` package, which does not exist. Mastra observability lives inside `@astropods/adapter-mastra` (the same package used for messaging); `serve(agent)` auto-wires OpenTelemetry whenever `OTEL_EXPORTER_OTLP_ENDPOINT` is set. The public page and CLI subsection both describe this real path.

### Navigation

A new top-level "Monitor your agents" section sits between "Make your agent discoverable" and "Guides" in `docs-public/fern/docs.yml`. It has an overview page and a "Frameworks" sub-section currently containing Mastra and Claude Agent SDK.

```yaml
- section: Monitor your agents
  contents:
    - page: Overview
      path: docs/pages/monitor-agents.mdx
    - section: Frameworks
      contents:
        - page: Mastra
        - page: Claude Agent SDK
```

### Pages

- **Overview** (`monitor-agents.mdx`) — explains that Astro auto-monitors model-provider calls, points at `https://astropods.com/insights`, and links to the framework guides. The "manual OpenTelemetry" subsection documents the three env vars injected into every container (`OTEL_EXPORTER_OTLP_ENDPOINT`, `ASTRO_AGENT_NAME`, `ASTRO_AGENT_BUILD`) and the local-dev caveat (endpoint is unset locally; instrumentation must guard on it).
- **Mastra** (`monitor-mastra.mdx`) — install `@astropods/observability-mastra`, attach via `observability()` on the `Mastra` constructor, optional `serviceName` / `endpoint` overrides.
- **Claude Agent SDK** (`monitor-claude-agent-sdk.mdx`) — drop-in replacement model: install `@astropods/adapter-claude-agent-sdk`, retarget existing imports, no other code changes. Version compatibility note: tracks `@anthropic-ai/claude-agent-sdk ^0.3.142`.

Each framework page ends with the same Verify section (deploy, send a message, traces appear within ~30s) and a troubleshooting bullet list.

### What's intentionally out of scope

- Specifics about the observability backend, collector internals, or per-account routing — agent authors don't need that context to instrument.
- LangChain / raw Anthropic / OpenAI stacks — no first-party adapter yet; the in-CLI `ast docs agent` guide already points those users at OpenInference / Traceloop.

## Migration

No action required. Pages are additive; the existing `ast docs agent` CLI guidance is unchanged.
