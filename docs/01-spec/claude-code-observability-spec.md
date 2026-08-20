# Claude Code Observability

## Summary

Astro captures observability from deployed agents today. This feature extends that to **local AI coding tools**, starting with **Claude Code**, running on an enterprise's developer machines. Astro streams usage telemetry (sessions, tokens, cost, tool activity, request traces) off each machine into its observability pipeline and surfaces it back to the enterprise as adoption, cost, and productivity insights.

**Claude Code already emits OpenTelemetry over OTLP**, so we don't build a plugin or scraper. We mint an org ingest key, the enterprise sets it as a forced environment via Anthropic managed settings, and `otel.astropods.ai` fronts the translation: it authenticates the key, resolves the account, and routes each OTel signal to its natural home.

This spec is collection-focused. Once data is streaming in, display is a straightforward fast-follow (see [Display](#display-fast-follow)).

### Scope decisions

- **Rollout:** Anthropic **managed settings**. The enterprise's admin sets the telemetry env once in the Anthropic admin console as a forced environment; it propagates to every developer with no per-machine action.
- **Capture:** Claude Code's **native OTel only**: metrics and `gen_ai` traces. No hooks, plugin, or transcript scraping in v1.
- **Storage:** route by signal. **Traces → a dedicated Langfuse project**; **metrics → Prometheus/Mimir**. Structured log events are deferred (see [Events / logs](#events--logs-deferred)).
- **Primary customer:** enterprise. Non-enterprise/self-serve accounts are a deliberate v2 (see [Non-enterprise path](#non-enterprise-path)).

### Non-goals (v1)

- Capturing prompt text, tool inputs/outputs, or source code (metadata only by default).
- Coverage of Claude Code sessions authenticated via Bedrock / Vertex / Foundry through the Anthropic console (see [Known limitations](#known-limitations)).
- Ingesting the structured log-event signal as its own store.
- The display layer beyond a sketch. Other coding tools (Codex, etc.) generalize later; only Claude Code is wired up now.

## Architecture

```
Enterprise developer machine
  └─ Claude Code  ──native OTLP (traces + metrics)──▶  otel.astropods.ai
        env pushed by Anthropic managed settings           (translation + routing)
        OTEL_EXPORTER_OTLP_ENDPOINT + Bearer <ingest key>            │
                                                    ┌────────────────┴────────────────┐
                                              auth org key → account, then route by signal
                                                    │                                 │
                                              traces (gen_ai)                     metrics
                                                    ▼                                 ▼
                                          dedicated Langfuse project          Prometheus / Mimir
                                          (per account)                       (tenant = account)
                                                    ▼                                 ▼
                                          existing trace UI                   dashboards (fast-follow)
```

**Tenancy model.** There is no "deployment" here. The unit is the **account** (the enterprise), identified by the **ingest key** in the request header. Claude Code attaches `user.email`, `user.id`, and `organization.id` to its signals, so each developer is identified from the payload, not from anything provisioned locally. Nothing on the developer's machine holds a per-user secret.

## Capture: what Claude Code emits

Once telemetry is enabled, Claude Code exports over OTLP with no instrumentation from us. Two of the three OTel signals matter for v1:

- **Metrics** (`claude_code.*`): `session.count`, `token.usage` (by input/output/cacheRead/cacheCreation), `cost.usage`, `lines_of_code.count`, `commit.count`, `pull_request.count`, `code_edit_tool.decision`, `active_time.total`. These are the stable bulk of the signal and drive the dashboards.
- **Traces** (enhanced-telemetry beta): links a prompt → API request(s) → tool executions as one distributed trace, using OTel `gen_ai` semantic conventions, which Langfuse ingests natively.

The third signal, structured log events (`user_prompt`, `api_request`, `tool_result`, `tool_decision`, …), is deferred (see [Events / logs](#events--logs-deferred)). The metrics already carry what the adoption/cost dashboards need.

Metrics export every ~60s and traces on turn boundaries (Claude Code defaults), so an action reaches the dashboard a couple of minutes later.

## Rollout: Anthropic managed settings

The enterprise's Anthropic org admin sets an `env` block in the Anthropic admin console (**Claude Code → Managed settings**). Claude Code fetches it at startup and refreshes it hourly on every developer's machine; it takes precedence over user/project settings, so an individual developer cannot override the OTLP destination or re-enable content logging.

The block we tell them to paste (endpoint and key filled in from their Astro dashboard):

```
CLAUDE_CODE_ENABLE_TELEMETRY        = 1
OTEL_METRICS_EXPORTER               = otlp
OTEL_TRACES_EXPORTER                = otlp
CLAUDE_CODE_ENHANCED_TELEMETRY_BETA = 1
OTEL_EXPORTER_OTLP_PROTOCOL         = http/protobuf
OTEL_EXPORTER_OTLP_ENDPOINT         = https://otel.astropods.ai
OTEL_EXPORTER_OTLP_HEADERS          = Authorization=Bearer <ASTRO_INGEST_KEY>
OTEL_METRICS_INCLUDE_SESSION_ID     = false
```

`OTEL_METRICS_INCLUDE_SESSION_ID=false` keeps `session.id` off metric datapoints, which would otherwise explode Prometheus label cardinality; per-session detail lives in traces instead. We leave `OTEL_LOG_USER_PROMPTS`, `OTEL_LOG_TOOL_DETAILS`, and `OTEL_LOG_RAW_API_BODIES` unset, which keeps prompts, tool I/O, and raw request bodies out of the stream (see [Privacy](#privacy-and-compliance)).

Each developer sees a one-time approval dialog the first time managed settings introduce a custom env var or new OTLP endpoint; the install guide calls this out.

## Ingest keys

We mint an account-scoped ingest key that the Anthropic admin sets as the forced telemetry environment. It is separate from user API keys: it rides on many machines, so it must be narrow (ingest-only, no read access) and revocable on its own.

New table in astro-server's database:

```sql
CREATE TABLE public.ingest_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id   uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name         text NOT NULL,                       -- admin-facing label
    token_hash   bytea NOT NULL,                      -- sha256(key); plaintext shown once
    token_prefix text NOT NULL,                       -- first chars, for display
    scope        text NOT NULL DEFAULT 'devtools-ingest',
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   uuid,
    last_used_at timestamptz,
    revoked_at   timestamptz
);
CREATE UNIQUE INDEX ingest_tokens_token_hash_idx ON public.ingest_tokens (token_hash);
```

Management endpoints (account-admin only):

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/accounts/:account/ingest-keys` | Create; returns plaintext key once |
| `GET` | `/api/v1/accounts/:account/ingest-keys` | List (prefix + metadata, never the secret) |
| `DELETE` | `/api/v1/accounts/:account/ingest-keys/:id` | Revoke |

The account-settings UI renders the pre-filled managed-settings block above with the key substituted in, ready to copy into the Anthropic console.

## Ingest & translation (otel.astropods.ai)

A public OTLP endpoint at `otel.astropods.ai` fronts the translation. Per request it:

1. **Authenticate:** read the `Authorization: Bearer` key, hash it, look it up in `ingest_tokens`, reject if missing or revoked. Cache key→account in memory (short TTL) so the hot path avoids a DB hit per batch. Update `last_used_at` off the hot path.
2. **Redact:** strip any prompt, completion, or tool-body attributes as defense in depth, even though managed settings keep them off at the source.
3. **Route by signal:** `gen_ai` traces to the account's dedicated Langfuse project (OTLP + per-account Basic auth); metrics to Mimir (Prometheus remote-write, tenant = account).

The translation layer must resolve per-account Langfuse credentials and the Mimir tenant, so it needs access to the account→credentials mapping astro-server already holds. Whether it runs as an OTel Collector gateway (`otlphttp` + `prometheusremotewrite` exporters) or a bespoke service is an infra decision; either way the auth, tenancy resolution, and two-way routing live here.

## Traces → Langfuse

Claude Code's enhanced traces use OTel `gen_ai` semantic conventions, which Langfuse's OTLP receiver understands, so the span/observation structure lands without custom mapping. Route them to a **dedicated Langfuse project per account**, separate from the account's agent-observability project, provisioned through the existing direct-Postgres path (`EnsureProject` extended with a project kind). Map tenancy onto Langfuse fields:

- `langfuse.user.id` ← `user.email`
- `langfuse.session.id` ← `session.id`

Traces are a beta Claude Code feature, so treat trace completeness as best-effort. The dashboards run on metrics and don't depend on traces.

## Metrics → Prometheus / Mimir

Claude Code's metrics are counters and gauges over time. That belongs in a time-series metrics store, not a trace store and not ClickHouse. The translation layer converts OTLP metrics to Prometheus remote-write and ships them to **Mimir** (Prometheus-compatible, horizontally scalable, multi-tenant).

- **Tenant = account**, via Mimir's `X-Scope-OrgID`. This isolates each enterprise's series.
- **Dimensions as labels:** `user` (`user.email`), `model`, and per-metric labels like `type` (token type) or `decision`. Keep the label set bounded.
- **Cardinality:** `session.id` is not a label (unbounded); it's dropped at the source via `OTEL_METRICS_INCLUDE_SESSION_ID=false`. Per-session and per-request detail lives in traces.
- **Cost** needs no computation on our side: Claude Code emits `cost.usage` directly, so it arrives as a metric.
- **Retention** is a Mimir config, set per contract.

## Events / logs (deferred)

Claude Code also emits structured log events. v1 does not ingest them as a separate store, because the metrics already expose what the adoption/cost dashboards need (tool decisions, tokens, cost, commits are all metrics). If raw per-event logs become necessary (detailed audit, per-prompt drill-down beyond traces), route them to **Loki**, the Grafana-stack log store that pairs with Mimir, rather than reintroducing ClickHouse. **Open item for infra.**

## Display (fast-follow)

Collection is the focus of this spec; display is a straightforward follow-on and is sketched here only to show the shape.

- **Metrics dashboards** query Mimir with PromQL (adoption, spend, tokens, tool-acceptance, commits, active developers), scoped by the account's Mimir tenant. Surfaced through Grafana or a **Developer Tools** section in astro-client that reads Mimir through an account-scoped astro-server proxy.
- **Traces** reuse the existing Langfuse-backed trace UI, pointed at the dedicated devtools project.
- **Access control:** per-developer breakdowns are gated to account admins (see [Privacy](#privacy-and-compliance)).

The detailed endpoint and client design is out of scope for this collection-focused spec.

## Privacy and compliance

This is enterprise employee telemetry, so we treat privacy as a first-class design constraint.

- **Metadata only by default.** We don't collect prompt text, tool inputs/outputs, or raw API bodies unless the org opts in: the managed-settings block leaves those flags off, and the translation layer strips such attributes regardless (defense in depth). Because managed settings outrank user settings, an individual developer cannot turn content logging on for our endpoint.
- **Admin-gated per-developer views.** Aggregate adoption/cost is viewable within the account; per-developer breakdowns are restricted to admins.
- **Documented data inventory.** The install guide states what is and isn't collected, so the enterprise can clear it with legal or works councils. Per-developer productivity telemetry can trigger employee-monitoring obligations (e.g. GDPR, EU works-council consultation); the guide names these up front.
- **Revocation & retention.** Ingest keys are revocable at any time; Mimir and Langfuse enforce retention; deleting an account cascades its ingest keys.

## Enterprise installation documentation

A new guide under `docs/04-guides/` (e.g. `instrument-claude-code.md`), written for an enterprise admin. Outline:

1. **What this does & what's collected:** the data inventory and privacy posture up front.
2. **Prerequisites:** developers must use Claude Code authenticated through the org's Anthropic account or org API key. Bedrock/Vertex/Foundry need the gateway path (see limitations).
3. **Generate an ingest key:** in the Astro dashboard (Account → Developer Tools → Ingest keys).
4. **Configure managed settings:** paste the pre-filled `env` block into the Anthropic admin console; explain the one-time per-developer approval dialog and the hourly propagation.
5. **Verify:** run Claude Code, confirm data appears in the Astro Developer Tools view within ~2 minutes.
6. **Staged rollout:** scope managed settings to a pilot group first if desired.
7. **Troubleshooting:** no data (key wrong/revoked, endpoint typo, telemetry flag off, unsupported auth provider); partial data (traces missing → enhanced-telemetry beta).

## Known limitations

- **Auth-provider coverage.** Anthropic-console managed settings only reaches Claude Code sessions on the org's Anthropic account. Sessions on Bedrock / Vertex / Foundry require a self-hosted Claude apps gateway to deliver managed settings; without one they aren't reached. This is the main motivator for a later self-config path.
- **Enhanced traces are beta.** The trace pipeline depends on a Claude Code beta flag; treat trace completeness as best-effort. Metrics (the bulk of the dashboards) are stable regardless.
- **~2 minute latency**, set by Claude Code's export intervals. This is usage observability; it won't drive live alerting.

## Non-enterprise path

Smaller accounts without an Anthropic enterprise admin console can't use managed settings. We defer them to v2, but build the pipeline to support them with no server changes: an `astro instrument claude-code` CLI command writes the same env config (plus a personal ingest key) into the developer's own Claude Code `settings.json`. It reuses the same endpoint and stores with a different distribution mechanism.

## Work breakdown

Ordered by dependency; several phases run in parallel.

1. **Infra (Mimir + Langfuse):** stand up (or reuse) Mimir with per-account tenancy; confirm Langfuse can host a dedicated devtools project per account; DNS/TLS for `otel.astropods.ai`; deploy target for the translation layer. (astro-infra)
2. **Ingest keys:** `ingest_tokens` table, store, management API, account-settings UI with copy-paste block.
3. **Translation layer:** OTLP receive at `otel.astropods.ai`, key auth, redact, route `gen_ai` traces → Langfuse and metrics → Mimir remote-write. After this, data lands.
4. **Langfuse devtools project:** extend `EnsureProject` with a project kind; first-ingest provisioning trigger.
5. **Display:** metrics dashboards over Mimir (PromQL) and the trace UI over the devtools Langfuse project.
6. **Docs:** the enterprise install guide, ready at GA.

## Configuration summary

**Translation layer (server-side):**

| Variable | Purpose |
|---|---|
| `MIMIR_REMOTE_WRITE_URL` | Prometheus remote-write endpoint for metrics |
| `LANGFUSE_DB_URL`, `LANGFUSE_SALT`, `LANGFUSE_ORG_ID`, `LANGFUSE_BASE_URL` | Reused for per-account Langfuse project provisioning & OTLP export |

Tenancy for both sinks (`X-Scope-OrgID` for Mimir, per-account project keys for Langfuse) is resolved from the ingest key, not configured statically.

**Developer machine (pushed via Anthropic managed settings):** see the [Rollout](#rollout-anthropic-managed-settings) block.

## Key decisions

**Native OTel over hooks/plugin.** Claude Code emits everything we need over OTLP already. A hooks/plugin approach would reimplement capture that ships for free and would add per-machine install surface. The native stream is richer (cost, token breakdown, tool decisions, traces) and enforceable org-wide via managed settings.

**Managed settings over CLI/plugin distribution.** For enterprise, the value is central enforcement with zero per-developer action: one admin config instead of N installs, and developers can't turn it off locally. The trade-off is the Anthropic-console auth-provider gap, which we accept for v1 and address later with the gateway or self-config paths.

**Route by signal: traces → Langfuse, metrics → Prometheus/Mimir.** The signals have different shapes and want different stores. Claude Code's `gen_ai` traces map natively onto Langfuse's model and reuse the existing trace UI. Its metrics are time-series counters/gauges that belong in Prometheus/Mimir; ClickHouse is a poor fit for metrics, and we don't expose a ClickHouse abstraction. `otel.astropods.ai` performs the translation so neither store's ingestion contract leaks to the client.

**Dedicated Langfuse project per account.** Claude Code traces land in their own project rather than the account's agent-observability project, keeping the two workloads and their retention/access cleanly separated.

**Account key for tenancy, payload attributes for identity.** The only secret on a developer's machine is an account-scoped, ingest-only, revocable key. Per-developer identity rides in the OTel attributes Claude Code already emits, so we avoid provisioning a credential per developer.

**Metadata only, redaction at ingest.** Prompts, code, and tool bodies stay off by default, and the translation layer strips them regardless, so the privacy posture doesn't depend on every developer's local config being correct.

**Events/logs deferred.** Metrics and traces cover v1; the structured log-event signal isn't ingested yet. If it's needed, Loki (not ClickHouse) is the home, keeping metrics and logs in one Grafana-compatible stack.
