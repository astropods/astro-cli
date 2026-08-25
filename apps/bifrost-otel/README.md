# bifrost-otel

An OpenTelemetry Collector distribution that turns Bifrost AI-gateway GenAI trace
spans into **Metronome usage events**. It is the delivery leg of LLM usage
metering — see [`../docs/architecture/16-llm-usage-metering.md`](../docs/architecture/16-llm-usage-metering.md).

## How it works

Bifrost's `otel` plugin exports one OTLP trace per LLM request. This collector
receives those traces and the custom `bifrostotel` exporter:

1. Selects the **billable span per trace** — the final successful attempt (highest
   `bifrost.retries`; a successful attempt wins ties). Retry/fallback attempts share
   one `bifrost.request.id`, so a retried request is billed **once**, never summed.
2. Maps its attributes to a Metronome event (`customer_id` = `bifrost.customer.id`,
   which is the Astro account ID / Metronome ingest alias; raw token counts as
   properties; `gen_ai.usage.cost` as a cross-check).
3. Batches ≤100 and POSTs to `{endpoint}/v1/ingest` with `transaction_id` =
   `bifrost.request.id` for idempotent (34-day) dedupe.

Durability comes from the collector framework: a **file-storage-backed sending
queue + retry** survives restarts and Metronome downtime (the Bifrost plugin itself
does not retry). No `batch` processor is used — it could split a trace across export
calls and break final-attempt selection.

## Layout

- `builder-config.yaml` — OCB manifest (otlp receiver, memory_limiter, filestorage, our exporter).
- `config/collector-config.yaml` — runtime pipeline config.
- `internal/exporter/bifrostotel/` — the custom exporter (Go).

## Build & run

```sh
make install-ocb   # one-time: installs the OpenTelemetry Collector Builder
make build         # → ./bin/bifrost-otel
make test          # exporter unit tests
METRONOME_API_KEY=… make run
```

## Config env vars

| Var | Default | Purpose |
|---|---|---|
| `METRONOME_API_KEY` | — (required) | Metronome bearer token |
| `METRONOME_ENDPOINT` | `https://api.metronome.com` | API base URL |
| `METRONOME_EVENT_TYPE` | `ai_gateway_llm_usage` | Metronome `event_type` |
| `QUEUE_STORAGE_DIR` | `/var/lib/bifrost-otel/queue` | persistent queue WAL dir |

## Deployment

CI builds `deployment/Dockerfile.bifrost-otel` and pushes `:latest` to
`${ECR_PREFIX}-bifrost-otel` whenever `apps/bifrost-otel/**` changes. Keel
watches that tag's digest and rolls the deployment, so a merge is the whole
deploy. The chart, values, and Terraform stay in astro-infra.
