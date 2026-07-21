# Summary

This PR updates the public (Fern) documentation to cover the **trace context** surface published across the messaging protocol and adapters.

# Design

Trace context is additive across both packages:

- A new `TraceContext` (W3C `traceparent` + optional `tracestate`) rides on `AgentResponse` and `PlatformFeedback` so any response — and the feedback referencing it — correlates back to its trace. The messaging SDK exposes it in Node (`@astropods/messaging`) and Python (`astropods_messaging`).
- The adapters add a `onTraceContext` / `on_trace_context` hook and a `createTraceparent` / `create_traceparent` helper. Framework adapters (Mastra, LangChain, JS variants) emit it automatically from their root span.

Documentation updates mirror this surface:

- Messaging SDK (Node/Python): `TraceContext` added to `AgentResponse` and `PlatformFeedback`, plus a Trace context section.
- Custom adapter (Node/Python): `onTraceContext` / `on_trace_context` hook and `createTraceparent` / `create_traceparent`.
- Mastra / LangChain guides: note on automatic trace-context propagation.
- New public changelog entry.

This PR is docs-only — it does not advance any submodule pointers; the trace context surface it documents was published to `adapters` and `messaging` independently.

# Migration

None. All package changes are additive; older SDKs and adapters keep working until upgraded.
