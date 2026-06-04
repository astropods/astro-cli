# Drop self-referential OTLP export spans in the collector

## Summary

The OTel collector was ingesting and forwarding spans that described the *act of exporting telemetry to itself*. Every agent export to `collector-sidecar:4318` produced an HTTP/gRPC client span which then flowed back through the collector and into Langfuse, inflating user-visible request counts and creating a small feedback loop on busy agents.

## Design

A new filter step runs at the top of the `astro` processor's `ConsumeTraces` and drops any span describing a call to an OTLP receiver before enrichment runs. Detection uses protocol-level signatures rather than the collector's hostname (which varies between sidecar DNS, dev `localhost:4318`, and in-pod loopback):

- **HTTP**: any of `url.full`, `url.path`, `http.url`, `http.target` contains `/v1/traces`, `/v1/metrics`, or `/v1/logs`. Both new and legacy semconv attribute keys are checked so the filter is independent of SDK/instrumentation version.
- **gRPC**: `rpc.service` starts with `opentelemetry.proto.collector.` (matches `…trace.v1.TraceService`, `…metrics.v1.MetricsService`, `…logs.v1.LogsService`).

Empty `ScopeSpans` and `ResourceSpans` are pruned after the drop so downstream exporters never see empty batches. The filter only runs on the traces pipeline — metrics and logs pipelines don't currently route through the `astro` processor, and the original symptom (inflated span counts) is span-only.

Dropping at the source rather than filtering at query/display time keeps Langfuse storage honest and avoids the inconsistency of different views showing different numbers.

## Migration

None. The filter is unconditional and matches on universal OTLP protocol constants; no configuration is needed.
