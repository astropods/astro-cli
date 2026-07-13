# Claude Code Observability Spec

## Summary

Astro captures observability from deployed agents but has no view into the AI coding tools developers run locally. This spec defines how Astro ingests usage telemetry from an enterprise's local Claude Code installs — sessions, tokens, cost, tool activity, and request traces — and surfaces it as adoption and cost insight, without shipping prompts or source code by default.

## Design

Claude Code already emits OpenTelemetry over OTLP, so the design ingests that native stream rather than building a plugin or scraper. Enterprises enable it centrally through Anthropic managed settings, which set an org ingest key as a forced environment pointing Claude Code at `otel.astropods.ai`. Tenancy comes from that account-scoped key in the request header; per-developer identity comes from OTel attributes Claude Code already attaches (`user.email`, `user.id`), so no per-developer secret lives on any machine.

`otel.astropods.ai` fronts the translation: it authenticates the key, resolves the account, redacts content attributes, then routes by signal. Claude Code's `gen_ai` traces go to a dedicated per-account Langfuse project (reusing the existing trace UI); its metrics go to Prometheus/Mimir with the account as the Mimir tenant. The structured log-event signal is deferred. This spec is collection-focused; display over Mimir (PromQL) and Langfuse is a fast-follow.

Key decisions: native OTel over hooks/plugin; managed settings over per-machine install; route by signal rather than one store, because `gen_ai` traces fit Langfuse but metrics belong in a time-series store (ClickHouse is not exposed); a dedicated Langfuse project per account; metadata-only capture with redaction at ingest.

## Migration

None. This is a design spec with no runtime change. Implementation is phased separately.
