# Astro Observation Spec

The Astro observation pipeline today flows from agent → OTel collector (with the custom `astro` processor) → Galileo. The collector enriches traces with `astro.agent.name`, `astro.agent.version`, and `astro.deployment.id` as OTel resource attributes. This is sufficient for forwarding telemetry but not for querying it back — Galileo's query API filters on its own column schema (`name`, `tags`, `external_id`, `log_stream_id`), not arbitrary OTel resource attributes.

This spec adds three capabilities:

1. **Annotation strategy** — map astro metadata into Galileo-queryable fields at the collector level
2. **Server proxy API** — endpoints on astro-server that proxy Galileo queries (server holds API keys)
3. **Aggregated dashboard** — per-agent metrics view in the client

## 1. Annotation Strategy

### Problem

The astro processor (`processor.go`) currently calls `enrichResourceAttributes` to stamp `astro.agent.name`, `astro.agent.version`, and `astro.deployment.id` on every resource. Galileo stores these as resource attributes but does not expose them as filterable columns in its search API. Galileo's filterable columns are: `name`, `tags`, `external_id`, `log_stream_id`, `model`, `status_code`.

### Approach — span-level enrichment

Enhance the astro processor to set span-level attributes that map to Galileo's queryable columns:

| Astro metadata | Where set | Galileo column | Purpose |
|---|---|---|---|
| Agent name | Span name prefix: `{agent_name}/{original_name}` | `name` | Filter traces by agent |
| Deployment ID | Span attribute `external_id` | `external_id` | Correlate to deployment |
| Agent name | Tag: `agent:{name}` | `tags` | Redundant tag-based filter |
| Agent version | Tag: `version:{version}` | `tags` | Version-based filtering |
| Deployment ID | Tag: `deployment:{id}` | `tags` | Deployment-based filtering |

The processor will apply these to root spans only (spans with no parent span ID). Non-root spans inherit the trace context and do not need independent annotation.

### Log stream mapping

Each deployment gets its own Galileo log stream. Galileo auto-creates log streams on first trace ingestion — when the collector sends traces with a `logstream` header value that doesn't exist yet, Galileo creates the stream. No explicit creation API call is needed.

The collector config already reads the log stream from an env var with a fallback:

```yaml
exporters:
  otlp_http/galileo:
    headers:
      Galileo-API-Key: ${env:GALILEO_API_KEY}
      logstream: ${env:GALILEO_LOG_STREAM:-default}
```

The spec applier sets `GALILEO_LOG_STREAM` to `{agent_name}-{deployment_id}` when creating the collector deployment. This scopes all traces from a deployment into a single log stream, enabling `log_stream_id`-filtered queries.

**Lifecycle:** Log streams are created implicitly on first trace and persist in Galileo. They are not deleted when a deployment is undeployed — historical data remains queryable. The proxy API discovers existing log streams at query time (see section 2).

### Processor changes

In `processor.go`, add a new function `enrichSpanAttributes` called from `ConsumeTraces` after `enrichResourceAttributes`:

```
enrichSpanAttributes(span, config):
  if span has no parent span ID:
    span.SetName(config.AgentName + "/" + span.Name())
    span.Attributes().PutStr("external_id", config.DeploymentID)
    tags = "agent:" + config.AgentName + ",version:" + config.AgentVersion + ",deployment:" + config.DeploymentID
    span.Attributes().PutStr("tags", tags)
```

No changes to `config.go` — existing `AgentName`, `AgentVersion`, and `DeploymentID` fields are sufficient.

## 2. Server Proxy API

### Problem

Galileo API keys and project IDs are server-side secrets. The client cannot call Galileo directly. The server needs proxy endpoints that accept agent-scoped queries and translate them to Galileo API calls.

### Prerequisites

Add to server config (`config.go`):

```
GalileoAPIEndpoint  string  // env: GALILEO_API_ENDPOINT, default: "https://api.galileo.ai"
```

`GalileoAPIKey` and `GalileoProject` already exist in `DeploymentConfig`.

### Endpoints

All endpoints are scoped to an agent by account and name. The server resolves the Galileo project ID from config and the log stream ID by querying Galileo's log stream search API (or caching the mapping from deployment records).

#### `GET /api/v1/agents/:account/:name/observability/metrics`

Returns time-bucketed aggregated metrics for the agent.

**Query params:**

| Param | Type | Default | Description |
|---|---|---|---|
| `start_time` | ISO 8601 | 24h ago | Start of time range |
| `end_time` | ISO 8601 | now | End of time range |
| `interval` | int (minutes) | 5 | Bucket interval |
| `deployment_id` | string | (all) | Filter to specific deployment |

**Server logic:**
1. Look up Galileo project ID from config
2. Resolve log stream ID — query `GET /v2/projects/{project_id}/log_streams/search` filtered by agent name, or use cached mapping from the deployment record
3. Call `POST /v2/projects/{project_id}/metrics/search` with `log_stream_id` filter + time range + interval
4. Return bucketed metrics

**Response shape:**

```json
{
  "buckets": [
    {
      "timestamp": "2025-01-15T10:00:00Z",
      "trace_count": 42,
      "avg_latency_ms": 380,
      "p95_latency_ms": 1100,
      "input_tokens": 12000,
      "output_tokens": 8000,
      "error_count": 2
    }
  ],
  "time_range": { "start": "...", "end": "..." },
  "interval_minutes": 5
}
```

#### `GET /api/v1/agents/:account/:name/observability/summary`

Returns high-level aggregated stats for the agent.

**Query params:**

| Param | Type | Default | Description |
|---|---|---|---|
| `start_time` | ISO 8601 | 24h ago | Start of time range |
| `end_time` | ISO 8601 | now | End of time range |
| `deployment_id` | string | (all) | Filter to specific deployment |

**Server logic:**
1. Same project ID + log stream resolution as `/metrics`
2. Call `POST /v2/projects/{project_id}/traces/search` with `include_counts: true`, filtered by log stream and time range
3. Aggregate response into summary stats

**Response shape:**

```json
{
  "total_traces": 1234,
  "time_range": { "start": "...", "end": "..." },
  "metrics": {
    "avg_latency_ms": 450,
    "p95_latency_ms": 1200,
    "total_tokens": 50000,
    "error_rate": 0.02,
    "traces_per_hour": 50
  }
}
```

#### `GET /api/v1/agents/:account/:name/observability/traces`

Returns a paginated trace list (summary only, no span-level drill-down in v1).

**Query params:**

| Param | Type | Default | Description |
|---|---|---|---|
| `start_time` | ISO 8601 | 24h ago | Start of time range |
| `end_time` | ISO 8601 | now | End of time range |
| `limit` | int | 50 | Page size |
| `offset` | int | 0 | Pagination offset |
| `status` | string | (all) | Filter by status code |

**Server logic:**
1. Same project ID + log stream resolution
2. Call `POST /v2/projects/{project_id}/traces/search` with filters
3. Return truncated trace list

**Response shape:**

```json
{
  "traces": [
    {
      "trace_id": "abc123",
      "name": "engineering-assistant/chat",
      "status": "OK",
      "latency_ms": 420,
      "tokens": { "input": 150, "output": 80 },
      "timestamp": "2025-01-15T10:05:00Z"
    }
  ],
  "total": 1234,
  "limit": 50,
  "offset": 0
}
```

### Log stream resolution

The server needs to map an agent name to a Galileo log stream ID. Two strategies:

1. **Preferred:** Store the log stream name (`{agent_name}-{deployment_id}`) in the deployment record when creating the collector. The proxy endpoint reads it from the deployment record.
2. **Fallback:** Query Galileo's `GET /v2/projects/{project_id}/log_streams/search` with agent name filter to discover stream IDs at query time. Cache results with a short TTL.

When `deployment_id` is not specified, the server queries across all log streams for the agent (multiple deployments may exist). When specified, the server filters to the single log stream `{agent_name}-{deployment_id}`.

## 3. Aggregated Dashboard

### Scope

Aggregated metrics only — no trace-level drill-down or span inspection in v1. The dashboard shows time-series charts and summary stats. Users who need trace details use the Galileo UI directly.

### Dashboard sections

1. **Header stats** — total traces, avg latency, error rate, total tokens (last 24h). Sourced from `/observability/summary`.
2. **Traces over time** — line chart of trace count per interval. Time range selector: 1h / 6h / 24h / 7d. Sourced from `/observability/metrics`.
3. **Latency distribution** — line chart of p50 and p95 latency over time. Same time range selector.
4. **Token usage** — bar chart of input vs output tokens per interval.
5. **Error rate** — line chart of error percentage over time.

### Client integration

**Query keys** — add to `src/api/queries/keys.ts`:

```
observabilityKeys:
  all:        ["observability"]
  metrics:    (account, name, params) => ["observability", "metrics", account, name, params]
  summary:    (account, name, params) => ["observability", "summary", account, name, params]
  traces:     (account, name, params) => ["observability", "traces", account, name, params]
```

**Query hooks** — new file `src/api/queries/observability.ts`:

- `useObservabilityMetrics(account, name, params)` — calls `/api/v1/agents/:account/:name/observability/metrics`, refetch interval 60s
- `useObservabilitySummary(account, name, params)` — calls summary endpoint, refetch interval 60s
- `useObservabilityTraces(account, name, params)` — calls traces endpoint, no auto-refetch

**API methods** — add to `src/lib/api.ts`:

- `getObservabilityMetrics(account, name, params): Promise<MetricsBucketResponse>`
- `getObservabilitySummary(account, name, params): Promise<SummaryResponse>`
- `getObservabilityTraces(account, name, params): Promise<TracesResponse>`

**Placement:** The dashboard renders as a tab within the agent detail page, alongside existing tabs (deployments, spec, etc.).

## 4. Configuration Changes

### Deployment spec — observability block extended

```yaml
observability:
  enabled: bool                   # deploy collector sidecar (default true)
  provider: string                # "galileo" (extensible)
  log_stream: string              # Galileo log stream name (default: "{source.name}-{deployment_id}")
```

`log_stream` is a new field. It is server-owned (not in the `editable` list) — the server generates it from the agent name and deployment ID during resolution. It appears in the template for visibility.

### Collector config

In `collector-config.yaml`, replace the hardcoded logstream header with an env var reference:

```yaml
exporters:
  otlp_http/galileo:
    headers:
      Galileo-API-Key: ${env:GALILEO_API_KEY}
      logstream: ${env:GALILEO_LOG_STREAM}
```

### Spec applier changes

In `spec_applier.go`, when building the collector deployment env vars, add:

```
GALILEO_LOG_STREAM = {agent_name}-{deployment_id}
```

This is set alongside the existing `GALILEO_API_KEY`, `GALILEO_PROJECT`, `ASTRO_AGENT_NAME`, `ASTRO_AGENT_VERSION`, and `ASTRO_DEPLOYMENT_ID` env vars.

### Server config addition

In `config.go`, add to `DeploymentConfig`:

```
GalileoAPIEndpoint  string  // env: GALILEO_API_ENDPOINT, default: "https://api.galileo.ai"
```

## 5. Files to Modify

| File | Change |
|---|---|
| `packages/astro-collector/internal/processor/astro/processor.go` | Add `enrichSpanAttributes` — span name prefix, `external_id`, `tags` |
| `packages/astro-collector/config/collector-config.yaml` | Use `${env:GALILEO_LOG_STREAM}` for logstream header |
| `apps/astro-server/internal/config/config.go` | Add `GalileoAPIEndpoint` field |
| `apps/astro-server/handlers/` | New observability proxy handlers (metrics, summary, traces) |
| `apps/astro-server/internal/k8s/spec_applier.go` | Pass `GALILEO_LOG_STREAM` env var to collector |
| `apps/astro-client/src/api/queries/keys.ts` | Add `observabilityKeys` factory |
| `apps/astro-client/src/api/queries/observability.ts` | New file — query hooks |
| `apps/astro-client/src/lib/api.ts` | Add observability API methods |
| `docs/01-spec/astro-deployment-spec.md` | Add `log_stream` to observability block |

## 6. Implementation Order

1. Collector annotation — span-level enrichment + log stream env var
2. Server proxy — config addition, Galileo client, proxy handlers
3. Client dashboard — query keys, hooks, API methods, dashboard UI

Each phase is independently deployable. Phase 1 improves queryability in Galileo's own UI immediately. Phase 2 enables programmatic access. Phase 3 builds the in-platform experience.

## Key Design Decisions

### Span-level over resource-level annotation

Galileo's search API exposes `name`, `tags`, and `external_id` as first-class filterable columns derived from span attributes. Resource attributes are stored but not queryable via the search API. Annotating at span level ensures data is queryable without Galileo-side schema changes.

### Root spans only

Annotating every span with name prefixes and tags would inflate trace data and create noise. Root spans are sufficient — Galileo's trace search operates on root spans, and child spans are retrieved by trace ID during drill-down (future v2).

### Server-side proxy over direct Galileo access

Galileo API keys must not be exposed to the client. The proxy also allows the server to enforce agent-level access control — a user can only query observability data for agents they own.

### Per-deployment log streams

Using `{agent_name}-{deployment_id}` as the log stream name provides natural isolation. Queries scoped to a deployment hit a single log stream. Cross-deployment queries for an agent can aggregate multiple streams. This avoids tag-based filtering for the most common query pattern (show me this deployment's traces).

### Aggregated-only dashboard in v1

Trace-level drill-down (span waterfall, attribute inspection) requires significant UI investment and duplicates what Galileo's own UI provides. v1 focuses on the metrics that inform deployment decisions: latency trends, error rates, token consumption, and throughput. Trace drill-down is a v2 concern.
