# astro-otel

OTLP ingest service for local AI coding tools (starting with Claude Code). Runs
alongside astro-server and shares its Postgres.

## What it does

Developer machines export OpenTelemetry over OTLP/HTTP to this service with an
account-scoped ingest key as a bearer token. Per request it:

1. **Authenticates** — `sha256(key)` looked up in `otel_ingest_tokens` (direct
   DB read, TTL-cached). No round-trip to astro-server, no ext_authz.
2. **Redacts** — strips prompt/completion/tool-body attributes as defense in
   depth.
3. **Routes by signal**
   - **traces →** the account's Langfuse project via Langfuse's OTLP endpoint,
     with per-account `Basic pk:sk` resolved from `account_langfuse` + KMS.
     Spans are tagged `langfuse.tags=["claude-code"]`.
   - **metrics →** VictoriaMetrics' native OTLP endpoint, stamped with an
     `astro.account_id` / `astro.source` resource attribute; `session.id` is
     dropped to bound label cardinality.

Both legs are OTLP pass-through, so no Prometheus remote-write translation.

## Why a separate service (not ext_authz in Envoy)

Auth needs the token DB and per-account Langfuse credentials (KMS). Contour's
external-auth is gRPC-only and can't be scoped without a TLS vhost, and a
collector can't resolve per-account exporter credentials. Doing it in-process
here keeps all of that in one place and off astro-server's request path.

## Config

| Env | Purpose |
|---|---|
| `DATABASE_URL` | astro-server Postgres (`otel_ingest_tokens`, `account_langfuse`) |
| `LANGFUSE_OTLP_ENDPOINT` | Langfuse OTLP base, e.g. `http://<langfuse-vpce>:3000/api/public/otel` |
| `VM_OTLP_ENDPOINT` | VictoriaMetrics OTLP push, e.g. `http://victoria-metrics-server.monitoring.svc.cluster.local:8428/opentelemetry/api/v1/push` |
| `TOKEN_CACHE_TTL` | key→account / creds cache TTL (default `60s`); also the max time a revoked key keeps working |
| `PORT` / `HOST` | listen addr (default `0.0.0.0:4318`) |

AWS credentials (IRSA) provide KMS `Decrypt` for Langfuse secret keys; without
them only plaintext-stored creds (dev) resolve.

## Endpoints

- `POST /v1/traces`, `POST /v1/metrics` — OTLP/HTTP protobuf (gzip supported)
- `GET /livez`, `GET /healthz`
