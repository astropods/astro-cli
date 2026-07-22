# Dev-tool prompt collection

## Summary

Astro collects usage telemetry (metrics, traces) from local AI coding tools but not the verbatim prompt/response content. This work defines and implements how that content is collected — to build a model-training corpus consumed offline by the `astro-models` pipelines — without adding a customer-facing surface.

## Design

Prompt and response text ride Claude Code's OTLP **logs** signal (`user_prompt` / `assistant_response` events), which the base telemetry block does not enable. Two constraints, both verified against captured Claude Code telemetry and Langfuse source, shape the design:

- The assistant **response is logs-only** — it never appears on a trace span under any configuration, so input/output pairs can only come from the logs signal.
- **Langfuse ingests only traces** — it exposes no OTLP `/v1/logs` endpoint — so the logs signal cannot be forwarded as-is.

The design bridges these:

- **Source.** Collection is opt-in via key-creation UI toggles ("Collect user prompts", "Store tool calls", both off by default) that append `OTEL_LOGS_EXPORTER=otlp` plus `OTEL_LOG_USER_PROMPTS=1` / `OTEL_LOG_TOOL_DETAILS=1` to the managed-settings block. Because the forced env lives in the customer's Anthropic console, the toggles shape the copyable block rather than acting as a live control.
- **Ingest + transform.** A new `POST /v1/logs` handler in astro-otel reshapes each content-bearing log record into a span and forwards it to the account's Langfuse `/v1/traces`. The transform is stateless: every Claude Code log record carries the parent interaction's `traceId`/`spanId` inline, so a synthesized span nests into the trace the prompt already belongs to — no cross-record join. Content maps onto Langfuse-native attributes (`langfuse.observation.type=generation`, `.input`/`.output`, `.model.name`, and `langfuse.trace.input`/`.output`), which Langfuse's ingestion reads directly.
- **Storage.** Content lands in the account's existing Langfuse project as observations under the interaction trace — no separate store — with retention set for corpus use.
- **Training access.** A dedicated internal variant of the eval trace→dataset pipeline, account-scoped to `claude-code`-tagged traces, upserts one dataset item per trace (`input`/`expectedOutput` from the trace's io fields) and serves it as a downloadable JSONL dataset. The transform's trace-level input/output map onto dataset items directly.

This revises an earlier spec that assumed a Langfuse logs passthrough and read Langfuse's internal event storage; both are replaced by the transform and the supported dataset path.

## Migration

None required. The `/v1/logs` endpoint is additive and no-ops unless a customer's admin enables the source flags; existing traces/metrics ingestion is unchanged.
