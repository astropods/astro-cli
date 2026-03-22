# Fix: Python Agent Trace Output Not Recorded

## Summary

Python agents using `opentelemetry-instrumentation-langchain` showed "Trace did not complete — no output recorded" in the monitor UI. The trace `output` field was always null in Langfuse despite the agent responding correctly.

## Design

The collector's astro processor only mapped Mastra-specific attributes (`mastra.*.input/output` → `langfuse.observation.input/output`) to Langfuse-recognized names. Python LangChain agents emit `traceloop.entity.input`/`traceloop.entity.output` (from the Traceloop/OpenLLMetry instrumentation library) which the processor ignored entirely.

The fix adds `mapLangchainAttributes()` to the processor, which maps `traceloop.entity.input/output` → `langfuse.observation.input/output` when those destinations are not already set. This runs before the existing `mapMastraTraceIO()` call, which then promotes `langfuse.observation.output` → `langfuse.trace.output` on root spans — the attribute Langfuse reads to populate the trace-level `output` field returned by `GET /api/public/traces`.

The no-overwrite behavior is consistent with the existing Mastra mapping: if `langfuse.observation.output` is already set (e.g. by a future instrumentation library that sets it directly), the traceloop mapping is skipped.

## Migration

No changes required for deployed agents. Rebuild and redeploy the collector image (`moon run deployment:collector`) to pick up the fix.
