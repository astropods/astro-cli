# Claude Code Observability — Implementation

Companion to [claude-code-observability-spec.md](./claude-code-observability-spec.md). The spec sets intent; this document maps it onto Astro's actual infrastructure and code, and corrects the spec's assumptions that don't hold here:

1. **No Mimir.** The metrics store is **VictoriaMetrics**, single-node on the **managed** cluster, no header/path multitenancy. Tenancy is a label.
2. **Langfuse per-account provisioning already exists, reused as-is.** `apps/astro-server/internal/langfuse` (`Provisioner.EnsureProject`, `Store`) already gives every account one Langfuse project. Claude Code traces land in **that same project**, distinguished by a `claude-code` tag — no separate project, no `account_langfuse` schema change.
3. **A standalone service (`astro-otel`), not Envoy ext_authz.** Auth needs the token DB and per-account Langfuse credentials. Contour's external-auth is gRPC-only and requires a TLS vhost to scope, and a stock collector can't resolve per-account exporter credentials. Doing auth **in-process** in a small service is simpler than every Envoy/Contour variant and keeps the token/credential logic in one place, off astro-server's request path.

## Scope

Ingest keys (astro-server + client, shipped), the `astro-otel` ingest service (built), and the infra to deploy it (preview first). Display is a fast-follow.

## Topology

`astro-otel` runs on the **primary** cluster, because that is where almost everything it needs is local: astro-server's Postgres (`otel_ingest_tokens`, `account_langfuse`) and Langfuse. The only remote dependency is VictoriaMetrics (managed cluster), reached over the **existing observability PrivateLink VPCE** that astro-server already uses for metric queries. This means a plain internet-facing **ALB** fronts it — no Contour, no ext_authz, no NLB.

```
Developer laptop (Claude Code)
  │  OTLP/HTTP protobuf, Authorization: Bearer <ingest key>
  ▼
ALB (internet-facing, ACM TLS)  →  astro-otel  (primary cluster)
     auth: sha256(key) → otel_ingest_tokens → account   (direct DB, TTL-cached)
     stamp astro.account_id + astro.source (redaction optional, off by default)
        ├─ traces  → Langfuse (in-cluster) /api/public/otel/v1/traces
        │            Basic pk:sk resolved from account_langfuse; tag "claude-code"
        └─ metrics → VictoriaMetrics (managed) /opentelemetry/api/v1/push
                     over the existing observability VPCE
```

**Tenancy unit is the account**, resolved from the ingest key — never from anything on the laptop. Per-developer identity rides in Claude Code's OTel attributes (`user.email`, `user.id`, `organization.id`).

## 1. Ingest keys (shipped)

Account-scoped, ingest-only, revocable. The credential set on developer machines; the string `astro-otel` validates.

`otel_ingest_tokens` in `sql/astro-server/schema.sql` (Atlas): `token_hash` is plain `sha256` (indexed, cache-friendly per-request lookup — bcrypt's per-hash salt would break that), `token_prefix` for display, `created_by text`, `revoked_at`. No `scope` column — ingest-only by construction.

Store `internal/ingesttoken`; handlers `handlers/otel_ingest_tokens.go`; account routes (`org:manage`) at `/api/v1/accounts/:account/otel-keys` (create/list/revoke). Create returns plaintext once + the ingest endpoint, and calls `EnsureProject` so the account's Langfuse project exists. Client UI: an "API Keys" section in personal and org settings that reveals the secret once and renders the managed-settings block; `OTEL_INGEST_ENDPOINT` config surfaces the real endpoint.

## 2. astro-otel service (built — `apps/astro-otel`)

A standalone Go OTLP/HTTP receiver. Own module, shares astro-server's Postgres. Per request:

1. **Authenticate** — `Bearer` token → `sha256` → `otel_ingest_tokens` lookup (via `internal/store`, TTL-cached; misses cached too so invalid-key floods can't hammer the DB). `last_used_at` stamped async. Invalid → 401; DB error → 503.
2. **Redact (optional, off by default)** — when `OTEL_REDACT_ATTRIBUTES=true`, strip prompt/completion/tool-body attributes as defense in depth. Disabled by default: managed settings keep that content off at the source, so this is an opt-in belt-and-suspenders control, not the primary guarantee.
3. **Route by signal** (below). Logs dropped in v1.

Both legs are **OTLP pass-through** — unmarshal (`go.opentelemetry.io/proto/otlp`), mutate attributes, re-marshal, forward — so there is no Prometheus remote-write translation. gzip request bodies supported; body capped at 16 MiB.

## 3. Traces → Langfuse (account's existing project)

`EnsureProject`/`Store` unchanged: one project per account, keyed `account_id`, secret stored as issued in `account_langfuse`. astro-otel resolves the account's `Basic base64(pk:sk)` at request time (`internal/store` reads `account_langfuse`, TTL-cached). It stamps `astro.account_id`/`astro.source` on the resource and `langfuse.tags=["claude-code"]` on spans, then POSTs OTLP to Langfuse `/api/public/otel/v1/traces` with that Basic auth. If the account has no project yet, traces are acked and dropped (provisioning happens at key creation, not here). Claude Code emits `gen_ai` spans, ingested natively.

## 4. Metrics → VictoriaMetrics

VM is `victoria-metrics-single` on the managed cluster, single tenant, no `X-Scope-OrgID`. astro-otel forwards OTLP metrics to VM's **native OTLP endpoint** (`/opentelemetry/api/v1/push`) over the observability VPCE, stamping `astro.account_id`/`astro.source` as resource attributes and dropping `session.id` (cardinality). Isolation is a label, surfaced at query time by an account-scoped astro-server PromQL proxy (same shape as `promquery.Client`'s `cluster` matcher) — a display fast-follow.

## 5. Ingress & TLS (infra — preview first)

A standard internet-facing **ALB Ingress** on the primary cluster, mirroring the `ai-gateway` pattern: `internet-facing`, HTTPS-only, ACM cert for the otel host (e.g. `otel-preview.astropod.ai`), `external-dns` annotation for the Route53 record. Backend = the `astro-otel` Service. No Contour, no ext_authz.

## Config summary

**astro-otel** (`apps/astro-otel`):

| Env | Purpose |
|---|---|
| `DATABASE_URL` | astro-server Postgres — `otel_ingest_tokens`, `account_langfuse` |
| `LANGFUSE_OTLP_ENDPOINT` | Langfuse OTLP base (in-cluster on primary: `http://langfuse-web.langfuse.svc.cluster.local:3000/api/public/otel`) |
| `VM_OTLP_ENDPOINT` | VictoriaMetrics OTLP push over the obs VPCE (`http://<obs-vpce>:8428/opentelemetry/api/v1/push`) |
| `TOKEN_CACHE_TTL` | key→account / creds cache TTL (default 60s; also max revoked-key lifetime) |

**astro-server**: `OTEL_INGEST_ENDPOINT` surfaced to the key UI (shipped).

**Terraform (astro-infra, preview)**: namespace + IRSA (DB); ECR repo `preview-astro-otel`; ExternalSecret for `DATABASE_URL`; Helm release + values; ACM cert + ALB Ingress + Route53 for the otel host; egress NetworkPolicy to the obs VPCE and Langfuse.

**Developer machine** (Anthropic managed settings): unchanged from the spec's Rollout block, `OTEL_EXPORTER_OTLP_ENDPOINT` → the otel host.

## Work breakdown

1. **Ingest keys** — schema, `internal/ingesttoken`, admin routes, settings UI. *(shipped)*
2. **astro-otel service** — OTLP receiver, DB auth + cache, redaction, per-account Langfuse routing, VM OTLP metrics. *(built — `apps/astro-otel`)*
3. **Infra (preview)** — ALB + cert + DNS for the otel host; Helm release + IRSA + ExternalSecret; VM OTLP reachability over the obs VPCE. (astro-infra)
4. **Display** — account-scoped PromQL proxy over VM; trace UI filtered to the `claude-code` tag. (fast-follow)
5. **Docs** — enterprise install guide. (GA)

## Open questions

- **Metric label surfacing** — VM's OTLP ingestion must promote `astro.account_id`/`astro.source` (resource attributes) to labels; confirm the VM flag or move them to datapoint attributes.
- **VM OTLP endpoint over the VPCE** — confirm the observability VPCE/NLB forwards `/opentelemetry/api/v1/push` (not just the query/`/api/v1/write` paths).
- **Redaction** — opt-in and off by default (`OTEL_REDACT_ATTRIBUTES`), since managed settings keep prompt content off at the source. If it's ever turned on broadly, the prompt-prefix list is duplicated in `astro-otel` and the `astro` collector processor — factor into a shared package then.
