## Summary

Ingestion keys stream local dev-tool telemetry (e.g. Claude Code) into Astro AI for every user on the key, with no way to carve out individuals. Some customers need per-person privacy exclusions. Each key can now carry a list of excluded emails: for an excluded person Astro AI keeps only usage metadata (tokens, model, cost, latency) and stores no full-text content — prompts, responses, and tool calls are dropped. The list is editable after creation without revoking or re-revealing the key.

Alongside the exclusions, the settings surface is reframed from "API Keys" to **Data Sources** — anticipating multiple external source kinds (Claude Code today, others soon) — and the Insights sources filter gains an always-visible **External** section with an "Add a source" entry point.

## Design

**Enforcement at ingest.** The metadata-vs-content split falls out of existing signal routing: metrics carry usage metadata (→ VictoriaMetrics); traces and logs carry content (→ Langfuse). Enforcement lives in `apps/astro-otel`, not at the source, so it holds regardless of a machine's local settings:

- **Metrics** forward untouched — excluded users' usage still counts.
- **Traces / logs** are filtered before forwarding: any span or log record whose `user.email` (span/record level, or the enclosing resource) is in the key's exclusion set is dropped. If nothing survives a batch, the receiver acks and skips the forward so exporters don't retry.

Exclusions ride the existing key-resolution read and TTL cache, so an edit propagates within the cache TTL (~60s) — the same lag model as a revoke — with no extra ingest-time query. The guarantee rests on `user.email` being stamped (Claude Code sets it per process), which is why matching checks both the record and its enclosing resource.

**Storage and API.** A per-key `excluded_emails text[]` on `otel_ingest_tokens`. Emails are normalized server-side (trim, lowercase, dedupe, validate, capped), which also makes ingest-time matching case-insensitive. A `PATCH /accounts/:account/otel-keys/:id/exclusions` endpoint edits the list in place and returns the normalized list, never the key. The list endpoint includes `excluded_emails`, so the edit dialog prefills from metadata alone — the plaintext key is required only at creation. Editing requires the same account-manage permission as key creation.

**Scope is deliberately per-key.** A person is excluded only on keys that list them; if a machine can present a different key, that key must list the email too. The UI copy states this and makes clear usage metadata is still collected.

**Surfacing.** The settings page becomes "Data Sources" with a per-source brand icon driven by a small kind registry (`data-source-kinds.ts`) resolved through the shared brand-icons pipeline (new `claude` and `claude-code` marks). The create flow reveals the key once, then offers collection toggles and the exclusions editor; a row menu re-opens exclusions without exposing the key. From Insights, the sources dropdown always shows the External section with an "Add a source" action (shown only to users who can create sources) that deep-links to the account's Data Sources settings with the create modal auto-opened.

## Migration

No action required. Existing keys default to an empty exclusion list (collect everything, unchanged).
