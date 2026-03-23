# Fix: Python Agent Trace Output Not Recorded

## Summary

Python LangChain/LangGraph agents showed "Trace did not complete — no output recorded" in Langfuse despite the agent responding correctly. Two root causes: the scaffold template pinned `astropods-adapter-langchain` to `0.1.0` which had no observability code, and the collector processor did not map LangGraph span attributes to Langfuse-recognized names.

## Design

**Adapter version (`astropods-adapter-langchain`):**

`0.1.1` adds `setup_observability()` which installs `LangchainInstrumentor`. The scaffold template is bumped to `>=0.1.1` so new agents get observability without manual setup.

**Collector processor:**

The astro processor only handled Mastra-specific attributes. Python LangChain/LangGraph agents emit `traceloop.entity.input`/`traceloop.entity.output` (Traceloop/OpenLLMetry) which the processor ignored.

`mapLangchainAttributes()` is added to map `traceloop.entity.input/output` → `langfuse.observation.input/output` per span. LangGraph's span structure places entity IO on a workflow child span (`traceloop.span.kind=workflow`) rather than the root span, so `mapMastraTraceIO()` — which promotes `langfuse.observation.*` → `langfuse.trace.*` — is now also called on workflow-kind spans in addition to root spans. This ensures `langfuse.trace.input/output` is set where Langfuse reads it for the trace-level output field.

## Migration

- Existing Python LangChain agents need to be rebuilt with `astropods-adapter-langchain>=0.1.1`. Run `astro build` and redeploy.
- Rebuild and redeploy the collector image (`moon run deployment:collector`) to pick up the processor fix.
- No changes required for TypeScript/Mastra agents.
