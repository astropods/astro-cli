# Agent OTel instrumentation guide

## Summary

`ast docs agent` previously covered Quick Start, tools, env vars, knowledge stores, and frontend agents, but said nothing about observability. Agent authors building outside the Mastra template (LangChain, Claude Agent SDK, raw Anthropic/OpenAI) had no guidance on how to make their agents show up in the platform's per-account observability backend, and the silent-failure modes are easy to miss.

This adds a focused **Instrumentation** section that names the right library for each common stack and points users at the auto-injected env vars.

## Design

### Where it lives

Between "Environment Variables" and "Project Structure" in `apps/astro-cli/cmd/docs/agent_instructions.md`. It also extends the env vars table with the three values the platform injects into the agent container automatically (`OTEL_EXPORTER_OTLP_ENDPOINT`, `ASTRO_AGENT_NAME`, `ASTRO_AGENT_BUILD`) so the rest of the section can refer to them by name.

### Framing

> Prefer an off-the-shelf instrumentation library for your stack — they emit richer spans (tool calls, agent steps, cost) than hand-written code and stay in sync with upstream SDK changes. When no library exists for your stack, emit manual spans using the OpenTelemetry GenAI semantic conventions or OpenInference conventions.

Libraries are the recommended path; manual is the documented fallback. Not the other way around.

### Per-stack content

- **Mastra** — one line; the existing template handles it.
- **LangChain / LangGraph (Python)** — Traceloop init snippet pointing at the platform endpoint.
- **Claude Agent SDK** — `@arizeai/openinference-instrumentation-claude-agent-sdk` with the `manuallyInstrument` constraint called out (the SDK is ESM-only and talks to the `claude` binary over IPC, so HTTP-level interception doesn't see it). Five-line snippet to get started; full re-export pattern lives in the OpenInference docs.
- **Raw Anthropic / OpenAI** — point at OpenInference or Traceloop equivalents; no inline code (HTTP-level instrumentations work out of the box).

### What's intentionally out of scope

- Specifics about the destination observability backend, per-account routing, or collector internals — agent authors don't need to know any of that to instrument.
- Local-dev caveats and verification rituals — those belong in operator docs, not the agent author's first encounter with instrumentation.
- Knowledge / ingestion sidecars that run LLM calls — currently rare enough that the agent-container-only guidance suffices; the few users in that situation today already know.

## Migration

No action required. New section is rendered by `ast docs agent`; existing docs unchanged.
