# Vercel AI SDK adapter

## Summary

Documents the new `@astropods/adapter-ai-sdk` package. The package exports two functions: `astroTelemetry()` for OpenTelemetry routing and `serve()` for messaging. Use them together or apart. This PR bumps the `modules/adapters` submodule and adds pages to the Fern docs and the in-CLI `ast docs agent` guide.

## Design

### Surfaces touched

- **Submodule:** `modules/adapters` bumped to the commit that adds `packages/ai-sdk` (adapter + telemetry helper + tests + README) and extends `@astropods/adapter-core`'s `getOrCreateAstroTracerProvider()` with a `register?: boolean` option.
- **Public Fern docs:** new "AI SDK" page under "Monitor your agents → Frameworks", listed first because the AI SDK leads JavaScript agent adoption. Linked from the overview page.
- **In-CLI `ast docs agent` guide:** new "Vercel AI SDK" subsection under "Instrumentation", between Mastra and the LangChain/Python entry.

### Wiring model

Spread `astroTelemetry()` into the agent's `experimental_telemetry`:

```typescript
const agent = new Agent({
  model: openai("gpt-4o"),
  experimental_telemetry: astroTelemetry(),
});
```

The helper returns `{ isEnabled: true, tracer }` when `OTEL_EXPORTER_OTLP_ENDPOINT` is set and `{ isEnabled: false }` otherwise. It builds the tracer from an unregistered `NodeTracerProvider` (via the new `register: false` option on `getOrCreateAstroTracerProvider()`), so the adapter does not modify the OpenTelemetry global. You can run Astro alongside another OTel provider without conflict.

`serve()` handles messaging. It does not touch OTel. Drop `serve()` to use the AI SDK adapter for `astroTelemetry()` alone, e.g. when the agent runs in a Next.js route handler or Lambda.

### What's intentionally out of scope

- Voice (`streamAudio`): the AI SDK ships separate experimental STT/TTS functions. The adapter does not stitch them into a single channel.
- A function-style `streamText` / `generateText` helper: if you have a hand-rolled loop, keep using `@astropods/adapter-core`.

## Migration

No action required. The page is additive and the submodule bump introduces a new package without changing existing adapters.
