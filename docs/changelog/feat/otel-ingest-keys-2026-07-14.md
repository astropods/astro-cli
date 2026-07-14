# OTel ingest keys

## Summary

Enterprises need to stream usage telemetry from local AI coding tools (starting with Claude Code) into Astro observability. Those tools already export OpenTelemetry over OTLP, so the only credential a developer machine needs is an account-scoped ingest key set as a forced telemetry environment. This change adds the management surface for that key: mint, list, and revoke account-scoped OTel ingest keys, plus the account-settings UI that renders a ready-to-paste managed-settings block.

This is the key-management slice of the larger Claude Code observability effort. The public ingest endpoint (`otel.astropods.ai`) and the metrics/trace routing are separate follow-ons; the keys minted here are the credential they authenticate.

## Design

**Tenancy is the account; the key is the only machine-side secret.** A key rides on many developer machines, so it is ingest-only (grants no read access) and revocable on its own. Per-developer identity is not provisioned locally — it rides in the OTel attributes the tool already emits.

**Storage.** A new `otel_ingest_tokens` table (account-scoped, `ON DELETE CASCADE`). The stored `token_hash` is a plain `sha256` of the plaintext — not bcrypt — because the future ingest endpoint must resolve a presented key by an indexed, cache-friendly hash lookup per batch, which a per-hash-salted scheme cannot support. Plaintext is returned once at creation and never persisted. There is no scope/permission column: the key's authority is fixed by construction.

**API.** Three account-admin endpoints (`org:admin`, since the key is forced org-wide):

| Method | Path |
|---|---|
| `GET` | `/api/v1/accounts/:account/otel-keys` |
| `POST` | `/api/v1/accounts/:account/otel-keys` |
| `DELETE` | `/api/v1/accounts/:account/otel-keys/:tokenID` |

Create returns the plaintext once alongside the ingest endpoint (from `OTEL_INGEST_ENDPOINT`) so the UI can render a copy-paste block. On create, the account's Langfuse project is ensured (idempotent, best-effort — a failure never blocks key creation) so the trace leg has a destination once the ingest endpoint ships.

**Langfuse reuse.** Traces route into the account's existing Langfuse project, distinguished by a `"claude-code"` tag rather than a separate project — no schema change to `account_langfuse`, no second project per account.

**UI.** An "API Keys" section under both personal-account settings and org settings (the org variant reuses the same panel with the org as the account, gated to admins). Creating a key reveals the secret once with a warning, plus the full managed-settings env block and copy buttons. Data access follows the app's TanStack Query conventions (query hooks + key factory, mutations invalidate on success).

## Migration

None. New table applies via the declarative schema; no changes to existing tables or endpoints. Set `OTEL_INGEST_ENDPOINT` on astro-server so the managed-settings block reflects the environment's real ingest URL (the UI falls back to `https://otel.astropods.ai` when unset).
