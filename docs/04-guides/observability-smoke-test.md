# Observability: telemetry paths and local verification

Astro has **two independent telemetry pipelines**. Keeping them distinct is the most important thing:

| | Deployed agents | Local AI coding tools (Claude Code) |
|---|---|---|
| Emitter | Agent process via `@astropods/adapter-*` (Mastra / LangChain / AI SDK / Claude Agent SDK) | Claude Code on a dev machine |
| Middle | `packages/astro-collector` (OTel Collector + custom `astro` processor), runs as a K8s sidecar | `apps/astro-otel` (standalone OTLP ingest service) |
| Traces | Langfuse | Langfuse (per account) |
| Metrics | **Langfuse** | **VictoriaMetrics** |
| Auth to backend | `LANGFUSE_AUTH_TOKEN` (base64 `pk:sk`), injected per deploy | Account ingest key (Bearer), resolved to per-account Langfuse creds |
| Read-back | astro-server reads Langfuse REST; shown on the astro-client Monitor tab | No dashboard surface yet |

> There is **no one-command local smoke test** anymore. The collector runs as a K8s sidecar (not part of `ast dev` compose), and `astro-otel` needs the shared Postgres. The closest real verification for each path is below. (The previous Galileo-based smoke test is gone: Galileo was replaced by Langfuse for traces and VictoriaMetrics for coding-tool metrics.)

## Path A - Deployed agents (adapter → collector → Langfuse)

### How it flows

- The agent adapter (`modules/adapters/packages/*`) builds an OTLP exporter **only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set** (`core/src/otel/provider.ts`). Locally that env var is unset, so agent telemetry is a silent no-op.
- astro-server injects `OTEL_EXPORTER_OTLP_ENDPOINT` (pointing at the collector sidecar, e.g. `http://astro-collector:4318`) and a per-deploy `LANGFUSE_AUTH_TOKEN` when observability is enabled for the deployment.
- The collector (`packages/astro-collector`, with the custom `astro` processor) enriches spans and exports **traces, metrics, and logs to Langfuse** at `${LANGFUSE_BASE_URL}/api/public/otel/v1/{traces,metrics,logs}`.
- astro-server reads the Langfuse REST API and serves `/api/v1/deployments/:id/observability/{summary,traces,metrics,...}`; the astro-client **Monitor** tab renders it.

### Verifying locally

Full path needs a local K8s cluster (Docker Desktop / kind), because the collector is a sidecar, not part of `ast dev` compose:

1. `moon run deployment:collector` (builds/tags `astropods/collector:latest`; on Docker Desktop/kind, import the image into the node's containerd and restart the pod - see [local-development.md](local-development.md)).
2. `ast-dev push` the agent to the local server; astro-server creates the deployment with the collector sidecar and the `OTEL_EXPORTER_OTLP_ENDPOINT` wiring.
3. Generate agent traffic (send chat messages).
4. Confirm landing: the astro-client **Monitor** tab, or `GET /api/v1/deployments/:id/observability/{summary,traces,metrics}` (these read Langfuse). You can also tail the sidecar: `kubectl -n <ns> logs <pod> -c astro-collector` - the `debug` exporter is in every pipeline.

No-K8s shortcut (proves emission and the `astro` processor without needing Langfuse):

- Run the collector binary with `packages/astro-collector/config/collector-config-dev.yaml` (debug exporter only), point a locally-run agent's `OTEL_EXPORTER_OTLP_ENDPOINT` at `http://localhost:4318`, send traffic, and confirm spans print to the collector's stdout.

## Path B - Claude Code (astro-otel → Langfuse + VictoriaMetrics)

### How it flows

- `apps/astro-otel` is a standalone OTLP ingest service: `POST /v1/traces`, `/v1/metrics`, `/v1/logs`, plus `GET /livez` and `/healthz`, on `0.0.0.0:4318`.
- Each request authenticates with `Authorization: Bearer <ingest key>` (its `sha256` is looked up in `otel_ingest_tokens`, TTL-cached via `TOKEN_CACHE_TTL`, default 60s).
- Traces are stamped (`langfuse.tags=["claude-code"]`, plus user/session identity) and forwarded to the account's Langfuse project (`LANGFUSE_OTLP_ENDPOINT`). Metrics are pushed to VictoriaMetrics (`VM_OTLP_ENDPOINT`).
- Ingest keys are minted in the astro-client **Settings → API Keys** page, which renders the managed-settings block to paste into Claude Code.

### Config (`apps/astro-otel`)

| Env | Default | Notes |
|---|---|---|
| `DATABASE_URL` | (required) | astro-server Postgres (`otel_ingest_tokens`, `account_langfuse`) |
| `LANGFUSE_OTLP_ENDPOINT` | - | Langfuse OTLP base; traces POSTed to `<base>/v1/traces` |
| `VM_OTLP_ENDPOINT` | - | VictoriaMetrics OTLP push endpoint |
| `TOKEN_CACHE_TTL` | `60s` | ingest-key cache TTL (also the max time a revoked key keeps working) |
| `OTEL_REDACT_ATTRIBUTES` | `false` | strip prompt/completion/tool-body attributes |
| `PORT` / `HOST` | `4318` / `0.0.0.0` | listen address |

At least one of `LANGFUSE_OTLP_ENDPOINT` / `VM_OTLP_ENDPOINT` must be set.

### Verifying locally

1. `moon run astro-otel:build` → `apps/astro-otel/bin/astro-otel`.
2. Run it with `DATABASE_URL` (astro-server Postgres) and `LANGFUSE_OTLP_ENDPOINT` and/or `VM_OTLP_ENDPOINT`. Check `GET /livez`.
3. Get an ingest key: astro-client **Settings → API Keys** (mints the key and ensures the Langfuse project), or for a pure-local harness insert an `otel_ingest_tokens` row (`token_hash = sha256(key)`) plus an `account_langfuse` row.
4. Point Claude Code at it via the managed-settings block (`OTEL_EXPORTER_OTLP_ENDPOINT=<astro-otel url>`, `OTEL_EXPORTER_OTLP_HEADERS=Authorization=Bearer <key>`), or send a hand-built OTLP `POST /v1/traces` with the Bearer header.
5. Confirm landing: HTTP 200; traces appear in the account's Langfuse project filtered to the `claude-code` tag; metrics appear in VictoriaMetrics. (There is no astro-client dashboard for this path yet.)

## Key files

- Adapters (emit): `modules/adapters/packages/{core/src/otel/provider.ts, langchain-js/src/instrumentation.ts, ai-sdk/src/telemetry.ts, mastra/src/observability.ts}`
- Collector: `packages/astro-collector/{config/collector-config.yaml, config/collector-config-dev.yaml, internal/processor/astro/processor.go}`
- astro-otel: `apps/astro-otel/{main.go, internal/ingest/ingest.go, internal/store/store.go, internal/config/config.go}`
- Read-back: `apps/astro-server/internal/langfuse/client.go` and the `/deployments/:id/observability/*` routes in `apps/astro-server/main.go`; `apps/astro-client/src/pages/agent-detail/AgentMonitor.tsx`
- Ingest keys: `apps/astro-server/handlers/otel_ingest_tokens.go`, `apps/astro-client/src/pages/settings/ApiKeysSettings.tsx`
