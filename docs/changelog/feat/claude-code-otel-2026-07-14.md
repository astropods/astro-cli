# Claude Code observability — trace identity + ingest-endpoint fix

## Summary

The `astro-otel` ingest service and account-scoped ingest keys shipped; local Claude Code now streams traces into each account's Langfuse project (tagged `claude-code`) and metrics into VictoriaMetrics. Validating a real capture surfaced two ingest-mapping gaps that make the traces hard to use, plus a stale endpoint in the key UI. This change fixes both.

## Design

**Trace identity mapping (`astro-otel`).** Claude Code emits identity on `user.email` / `session.id`, but Langfuse populates a trace's `userId`/`sessionId` from `langfuse.user.id` / `langfuse.session.id`. Left unmapped, Langfuse fell back to the opaque hashed `user.id` for `userId` and left `sessionId` empty — so a trace couldn't be attributed to a developer by email or grouped into a coding session. `astro-otel` now promotes `user.email → langfuse.user.id` and `session.id → langfuse.session.id` per span, alongside the existing tag/redaction. Token-usage remap (`gen_ai.usage.*`) is deliberately out of scope: aggregate cost/tokens come from the VictoriaMetrics metrics leg, and Langfuse can't price a custom model regardless.

**Ingest-key endpoint is server-driven, not hardcoded.** The key UI rendered a managed-settings block with a hardcoded fallback host when `OTEL_INGEST_ENDPOINT` was unset — and no environment set it, so every environment hit that fallback. The ingest host differs per environment (preview and prod use different domains), so the client no longer bakes in a real URL: the endpoint comes solely from the server's `OTEL_INGEST_ENDPOINT`, and the block shows an explicit unset placeholder when it's missing. Each environment sets `OTEL_INGEST_ENDPOINT` in its astro-server helm values (alongside `PROMETHEUS_URL`).

## Migration

None for existing surfaces. In-place ingest fixes only. Deployments should set `OTEL_INGEST_ENDPOINT` on astro-server per environment so the ingest-key block shows the environment's real endpoint.
