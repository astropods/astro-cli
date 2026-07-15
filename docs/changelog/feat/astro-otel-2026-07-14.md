# astro-otel — OTLP ingest service for coding tools

## Summary

The account-scoped ingest keys shipped earlier are the credential; this adds the service that actually receives telemetry from local AI coding tools (starting with Claude Code) and lands it in Astro observability. Developer machines export OpenTelemetry over OTLP/HTTP with an ingest key as a bearer token; `astro-otel` authenticates the key, then routes each signal to its store.

## Design

**A standalone service, not Envoy/Contour auth.** Authenticating a machine bearer token and resolving per-account Langfuse credentials both need the token DB and KMS. Contour's external-auth is gRPC-only and requires a TLS vhost to scope, and a stock collector can't resolve per-account exporter credentials. Doing auth **in-process** is simpler than every edge-proxy variant and keeps the token/credential logic in one place, off astro-server's request path.

**Runs on the primary cluster.** That is where the token DB, KMS, and Langfuse are all local. The only remote dependency is VictoriaMetrics (managed cluster), reached over the observability PrivateLink VPCE astro-server already uses. So it is fronted by a plain internet-facing ALB — no Contour, no ext_authz, no NLB.

**Per request:** authenticate (`sha256(key)` → `otel_ingest_tokens`, TTL-cached, misses cached too), redact prompt/tool-body attributes, then route:
- **traces →** the account's existing Langfuse project via its OTLP endpoint, with per-account `Basic pk:sk` resolved from `account_langfuse` + KMS decrypt; spans tagged `langfuse.tags=["claude-code"]`.
- **metrics →** VictoriaMetrics' native OTLP endpoint, stamped with `astro.account_id`/`astro.source`; `session.id` dropped to bound cardinality.

Both legs are OTLP pass-through (unmarshal → mutate → forward), so there is no Prometheus remote-write translation.

**Redaction is opt-in, off by default.** Stripping prompt/completion/tool-body attributes is gated behind `OTEL_REDACT_ATTRIBUTES` (default `false`) — managed settings already keep that content off at the source, so in-service redaction is defense in depth, not the primary guarantee.

**CI:** the service builds, publishes (multi-arch, by-digest → `:sha`/`:latest`), and tests through the existing preview/prod pipelines and the Go test matrix.

## Migration

None. New service, new endpoint. Its preview infrastructure (namespace, IRSA, ExternalSecret, ALB, DNS, ECR repo) lives in the astro-infra repo. Deploying it requires setting `DATABASE_URL`, `LANGFUSE_OTLP_ENDPOINT`, and `VM_OTLP_ENDPOINT`; without a Langfuse project for an account, its traces are acked and dropped (projects are provisioned at ingest-key creation).
