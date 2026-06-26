# Reconcile CLI observability docs with public docs

## Summary

The CLI's embedded agent guide (`ast docs agent`, sourced from `apps/astro-cli/cmd/docs/agent_instructions.md`) had drifted from the public Fern docs and from the adapter packages shipped in `modules/adapters`. Its Instrumentation section listed no version constraints and gave no single view of which adapters exist, even though first-party adapters now auto-wire OpenTelemetry through `serve()`.

## Design

The Instrumentation section is the single place the CLI describes observability, so the reconciliation is contained there:

- **Adapter inventory** — a new "Available adapters" list enumerates the documented packages (mastra, ai-sdk, claude-agent-sdk, and the two `*-core` packages), each tagged with language, framework, and how tracing is wired. This gives `ast docs agent` readers an at-a-glance map.
- **First-party over manual** — the guiding line shifts from "pick an off-the-shelf instrumentation library" to "prefer a first-party adapter; `serve()` auto-wires OTEL when `OTEL_EXPORTER_OTLP_ENDPOINT` is set and is a no-op locally." Manual OpenTelemetry and OpenInference/Traceloop remain the fallback for stacks without an adapter.
- **Version truth** — version constraints now come from the packages themselves: `ai >= 6.0.0`, `@mastra/core >= 1.14.0`, Python 3.10+ for the core package, and the unchanged `@anthropic-ai/claude-agent-sdk ^0.3.142` range for the Claude Agent SDK drop-in.

The LangChain / LangGraph adapters are intentionally left out of the CLI guide for now; they are not yet promoted in the user-facing docs.

Command vocabulary (`ast project start` / `ast project configure`) is left untouched to stay consistent with the rest of the embedded guide and `ast docs help`.

While reconciling, the public Claude Agent SDK guide (`monitor-claude-agent-sdk.mdx`) was found pinning a stale install version (`^0.3.0`); it is bumped to `^0.5.0` to match the shipped package. The wrapped `@anthropic-ai/claude-agent-sdk ^0.3.142` range is unchanged and stays correct.

## Migration

None. Documentation only — the change updates an embedded Markdown file rendered by `ast docs agent` and a public docs page. No CLI behavior, flags, or output change.
