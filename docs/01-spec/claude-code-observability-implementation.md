# Claude Code Observability — Implementation

Companion to [claude-code-observability-spec.md](./claude-code-observability-spec.md). The spec sets intent; this document maps it onto Astro's actual infrastructure and code, and corrects two spec assumptions that don't hold here:

1. **No Mimir.** The metrics store is **VictoriaMetrics**, deployed **single-node per cluster** with no header/path multitenancy. Tenancy is label-based and enforced at query time, not by `X-Scope-OrgID`.
2. **Langfuse per-account provisioning already exists, and we reuse it as-is.** `apps/astro-server/internal/langfuse` (`Provisioner.EnsureProject`, `Store`, KMS envelope) already gives every account one Langfuse project. Claude Code traces land in **that same project** — we do **not** create a separate "devtools" project. Claude Code traces are distinguished from agent traces by a tag, not by a project boundary.

## Scope

Collection only. Ingest keys, the translation/ingest service, routing traces into the account's existing Langfuse project, and the VictoriaMetrics metrics leg. Display is a fast-follow (a PromQL proxy sketch is included because it constrains the metrics label design).

## Topology

Everything net-new runs on the **primary cluster**, because that is where the state lives: astro-server's Postgres (`otel_ingest_tokens`, `account_langfuse`), the KMS key, Langfuse (and its Postgres), and the query path. Developer laptops are not in any managed cluster, so — unlike agent telemetry — this traffic does not originate in-cluster and cannot use the managed-cluster trace-router path.

```
Developer laptop (Claude Code)
  │  OTLP/HTTP protobuf, Authorization: Bearer <ingest key>
  ▼
otel.astropods.ai            (Contour HTTPProxy + cert-manager TLS, primary cluster)
  ▼
ingest service (new, Go)     auth → redact → split by signal
  ├─ traces  → resolve account's Langfuse project (Store.Get + KMS), tag "claude-code" →
  │            OTLP/HTTP to Langfuse /api/public/otel/v1/traces (Basic pk:sk)
  └─ metrics → stamp account_id label → Prometheus remote-write →
               devtools VictoriaMetrics (single-node, primary cluster)
```

**Tenancy unit is the account.** It is resolved from the ingest key in the request header, never from anything on the laptop. Per-developer identity rides in Claude Code's OTel attributes (`user.email`, `user.id`, `organization.id`).

## 1. Ingest keys

Account-scoped, ingest-only, revocable. Separate from user API keys and deploy tokens because the key rides on many machines.

### Schema

Add to `sql/astro-server/schema.sql` (Atlas diffs and applies; there is no migration file to author):

```sql
CREATE TABLE public.otel_ingest_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id   uuid NOT NULL REFERENCES public.accounts(id) ON DELETE CASCADE,
    name         text NOT NULL,
    token_hash   bytea NOT NULL,                 -- sha256(plaintext); shown once
    token_prefix text NOT NULL,                  -- for display
    created_at   timestamptz NOT NULL DEFAULT now(),
    created_by   uuid,
    last_used_at timestamptz,
    revoked_at   timestamptz
);
CREATE UNIQUE INDEX otel_ingest_tokens_token_hash_idx ON public.otel_ingest_tokens (token_hash);
CREATE INDEX otel_ingest_tokens_account_idx ON public.otel_ingest_tokens (account_id);
```

`token_hash` is a plain `sha256` (not bcrypt): the ingest path verifies it per batch and must be cache-friendly; the key is high-entropy so a slow hash buys nothing.

The token's authority is fixed by construction — it can only push OTLP telemetry (traces + metrics) for its account and grants no read access — so there is no `scope`/permission column. If a narrower key is ever needed (e.g. metrics-only), add a typed column then and enforce it in the ingest service; don't ship a placeholder now.

### Store & API

New `internal/ingesttoken` package (mirror `internal/langfuse/store.go` shape); handlers in `handlers/otel_ingest_tokens.go`. Routes under the existing account-admin group in `main.go` (`ResolveAccount` + `RequireAccountPermission(accountStore, "org:admin")`) — admin-only, since the key is forced org-wide onto developer machines:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/v1/accounts/:account/otel-keys` | Create; returns plaintext once; **ensures the account's Langfuse project exists here** via `EnsureProject` (see §3) |
| `GET` | `/api/v1/accounts/:account/otel-keys` | List (prefix + metadata) |
| `DELETE` | `/api/v1/accounts/:account/otel-keys/:id` | Revoke (set `revoked_at`) |

Plaintext format `astotel_<base32(random 20 bytes)>`; store `sha256` + first 8 chars as prefix.

## 2. Ingest service (otel.astropods.ai)

A **standalone Go service**, not a stock OTel Collector. The deciding constraint: both legs need **per-request, per-account** resolution against astro-server's DB and KMS — the Langfuse Basic-auth credential and the metrics tenancy label. A vanilla collector's exporters are statically configured and cannot resolve a credential per account for a growing account set. A bespoke service imports the code that already does this (`internal/langfuse`, `internal/ingesttoken`, `internal/envelope`) directly.

> Alternative considered: extend the existing custom collector distro (`packages/astro-collector`) with an auth extension + custom routing exporter. Rejected for v1 — the credential/tenant resolution still has to be hand-written, and doing it in a plain HTTP service is less surface than a collector component. Envoy/Contour may still front the service for TLS and rate limiting; auth stays in the service so one component owns key→account.

Per request:

1. **Authenticate.** Read `Authorization: Bearer`, `sha256` it, look up in `otel_ingest_tokens`, reject if missing or `revoked_at` set. Cache `hash→account` in memory with a short TTL so the hot path skips the DB. Update `last_used_at` asynchronously (best-effort, coalesced).
2. **Redact.** Strip prompt/completion/tool-body attributes as defense in depth. Reuse the prefix list already in `packages/astro-collector/internal/processor/astro/processor.go` (`gen_ai.prompt`, `gen_ai.completion`, `gen_ai.input/output`, `langfuse.*.input/output`, …). Managed settings keep these off at source; this guarantees it regardless of a laptop's local config.
3. **Split by signal** and route (§3, §4). Logs are dropped in v1 (deferred).

OTLP/HTTP protobuf only (matches the managed-settings `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`). No gRPC receiver needed for v1.

## 3. Traces → Langfuse (the account's existing project)

### The mechanism, reused unchanged

`Provisioner.EnsureProject` creates a project + `api_keys` row directly in Langfuse's Postgres (the management API needs an enterprise license we don't have), KMS-encrypts the secret key, and stores it in `account_langfuse` keyed `PRIMARY KEY (account_id)`. `Store.Get` + `decryptSecretKey` return the plaintext `pk`/`sk`. **Langfuse routing = sending that project's `Basic base64(pk:sk)`** on the OTLP request — there is no tenant header.

**No schema change. No `kind` column. No second project.** Claude Code traces go to the account's one project. `EnsureProject` and `Store` are used exactly as they are today.

### Distinguishing Claude Code traces

Because agent traces and Claude Code traces share the project, the ingest service tags every Claude Code span with `langfuse.tags = ["claude-code"]` (the trace-router already stamps `langfuse.tags` for agents, so this is the same field). The trace UI and any queries filter on that tag. Access control and retention are therefore uniform across both trace kinds within the account — acceptable here because the account already owns and can see its own project; if per-developer Claude Code data ever needs stricter gating than agent traces, that gating happens in the astro-client/proxy layer, not by splitting the Langfuse project.

### Provisioning trigger

**At ingest-key creation**, not first-ingest. The spec floats first-ingest, but that puts a Langfuse-Postgres write and a provisioning race on the hot path. Key creation is an authenticated admin action that happens once before any telemetry flows — call `EnsureProject` there (idempotent: it returns the existing project if the account already has one) so the credential is ready when the first span arrives, off the ingest path.

### Attribute mapping

Set on spans in the ingest service before export (spec fields):

- `langfuse.user.id` ← `user.email`
- `langfuse.session.id` ← `session.id`
- `langfuse.tags` ← append `"claude-code"`

Claude Code emits `gen_ai` semantic-convention spans, which Langfuse's OTLP receiver ingests natively — this is an attribute rename, not a schema mapping. The managed-cluster trace-router already does the analogous `transform/langfuse_attrs` step; model it on that.

### Export target

Langfuse's OTLP endpoint at `<langfuse-base>/api/public/otel/v1/traces` with `Authorization: Basic base64(pk:sk)` of the account's project. On the primary cluster the service reaches Langfuse's service/NLB directly (config via the existing `LANGFUSE_BASE_URL`; the collector-facing `LANGFUSE_BASE_URL_EXT` already exists for this split). Traces are a Claude Code beta — treat completeness as best-effort; dashboards do not depend on them.

## 4. Metrics → VictoriaMetrics

### Reality of the store

VictoriaMetrics is `victoria-metrics-single`, one node per cluster in the `monitoring` namespace, remote-write at `:8428/api/v1/write`, ~30d retention. **No `X-Scope-OrgID`, no vminsert, no path tenancy.** Existing isolation is label-based: agent metrics carry `cluster` and `customer_id`, and astro-server (`internal/promquery`) injects a `cluster` label matcher into every query.

### Design

Laptops are not in a managed cluster, so devtools metrics need a target on the primary side. Stand up (or reuse an infra-side) **dedicated single-node VictoriaMetrics for devtools metrics**, with its own retention set per contract.

- **Tenancy = an `account_id` label** stamped by the ingest service on every series, mirroring the existing `customer_id` pattern. There is no cryptographic tenant boundary in single-node VM; **isolation is enforced at query time** by the read proxy (below), and the VM instance is never exposed outside the cluster — only the ingest service writes and only astro-server reads.
- **Cardinality.** `session.id` must not become a label; it is already dropped at source via `OTEL_METRICS_INCLUDE_SESSION_ID=false`, and the service drops it if present. Keep labels bounded: `account_id`, `user` (`user.email`), `model`, and per-metric dims like `type` (token type) or `decision`. Per-session detail lives in traces.
- **Cost** arrives directly as `cost.usage`; no computation needed.
- **Write path.** Convert OTLP metrics to Prometheus remote-write and POST to the devtools VM `/api/v1/write`. (`prometheus.NewClient` in `promquery` is read-only; the write encoder is new — a small remote-write marshaller or the OTel `prometheusremotewrite` translation.)

### Read proxy (display fast-follow, but design now)

Reads go through an account-scoped astro-server proxy that injects `account_id="<account>"` into every PromQL query — exactly how `promquery.Client` injects `cluster` today. The VM instance stays internal; clients never hit it directly. Per-developer breakdowns are gated to account admins (privacy).

## 5. Ingress & TLS

`otel.astropods.ai` is a new **public, internet-facing** host (laptops, not in-cluster pods). Route it through Contour on the primary cluster: a `HTTPProxy` for the host with a cert-manager-issued TLS cert, backend = the ingest service. This is the front-door ALB → Envoy (Contour) pattern already used for `*.agents.<domain>`; add the host and a DNS record. Envoy can optionally rate-limit here, but authentication stays in the ingest service.

## Config summary

Ingest service (new deployment, primary cluster):

| Variable | Purpose |
|---|---|
| `ASTRO_DB_URL` | astro-server Postgres — `otel_ingest_tokens`, `account_langfuse` |
| `LANGFUSE_DB_URL`, `LANGFUSE_SALT`, `LANGFUSE_ORG_ID` | reuse `NewProvisioner` for the account's project provisioning |
| `LANGFUSE_BASE_URL` / `LANGFUSE_BASE_URL_EXT` | OTLP export target for traces |
| (KMS config) | reuse `envelope` to decrypt the account project's secret key |
| `DEVTOOLS_VM_REMOTE_WRITE_URL` | devtools VictoriaMetrics `/api/v1/write` |
| `INGEST_TOKEN_CACHE_TTL` | key→account cache TTL |

Provisioning happens in astro-server (key-create handler) using the config it already has; the ingest service needs the Langfuse config only if it also provisions (keep provisioning in astro-server, so the service is read-only against `account_langfuse`).

Developer-machine env (Anthropic managed settings) is unchanged from the spec's [Rollout](./claude-code-observability-spec.md#rollout-anthropic-managed-settings) block, pointing `OTEL_EXPORTER_OTLP_ENDPOINT` at `https://otel.astropods.ai`.

## Work breakdown

Ordered by dependency; several run in parallel.

1. **Ingest keys** — `otel_ingest_tokens` in `schema.sql`, `internal/ingesttoken` store, three account-admin routes, account-settings UI with the pre-filled managed-settings block. (astro-server, astro-client)
2. **Langfuse wiring** — no schema or `EnsureProject` change; call `EnsureProject` in the key-create handler to guarantee the account's project exists. Tag Claude Code spans `"claude-code"` in the ingest service. (astro-server)
3. **Ingest service** — OTLP/HTTP receiver, key auth + cache, redaction, attribute mapping, trace export to Langfuse, metric remote-write to VM. After this, data lands. (new service)
4. **Infra** — devtools VictoriaMetrics single-node (Helm + Terraform, mirroring `victoria-metrics.yaml.tpl`); `otel.astropods.ai` HTTPProxy + cert + DNS; deploy target for the ingest service. (astro-infra)
5. **Display** — account-scoped PromQL proxy over devtools VM; existing trace UI over the account's Langfuse project, filtered to the `"claude-code"` tag. (fast-follow)
6. **Docs** — enterprise install guide under `docs/04-guides/`. (GA)

## Open questions

- **Devtools VM placement** — dedicated single-node on primary vs. reuse of the infra-cluster Prometheus/VM. Dedicated keeps retention and blast radius separate; decide with infra.
- **Provisioning ownership** — keep `EnsureProject` calls in astro-server (recommended: one writer to `account_langfuse`) vs. letting the ingest service self-provision on first unseen account. Former avoids a hot-path race.
- **Redaction parity** — factor the prompt-attribute prefix list out of `packages/astro-collector` into a shared package so the ingest service and the collector cannot drift.
