# astro-server: point AI Gateway key issuance at Bifrost

## Summary

The AI Gateway that astro-server provisions per-tenant keys against is migrating
from LiteLLM to Bifrost (now serving the gateway hostname). This repoints
astro-server's `internal/aigateway` integration at Bifrost's governance API. The
tenant-facing contract is unchanged — apps still use the OpenAI SDK with a
`base_url` and a bearer key, and call the same friendly model names — so this is
a control-plane change (how keys are minted, scoped, and revoked), not a
data-plane one.

## Design

**Client.** The client now speaks Bifrost's governance API: `POST` and
`DELETE /api/governance/virtual-keys` authenticated with an HTTP Basic admin
header, replacing LiteLLM's `/key/generate` + `/key/delete` and master-key
bearer auth. Config splits the single gateway URL into a public `base_url` (what
tenants reach, written into tenant Secrets) and an in-cluster admin URL for the
governance API — Bifrost's public data host doesn't route `/api`, and the admin
host is network-gated, so issuance always goes in-cluster. `AI_GATEWAY_MASTER_KEY`
is replaced by `AI_GATEWAY_ADMIN_URL` + `AI_GATEWAY_ADMIN_AUTH` (a pre-built
`Basic base64(user:pass)` header).

**Per-account customer + shared budget.** Each account maps to a Bifrost
*customer*. On first key issuance the account's customer is created once (named
by account-id, carrying a monthly budget) and its server-generated id is
persisted on `accounts.bifrost_customer_id`; subsequent mints reuse it. Every
virtual key — deployment and dev — is attached to that customer and inherits a
single per-account budget, rather than each key carrying its own. Keys grant the
Bedrock provider via `key_ids: ["*"]`. The `Provisioner` gains a small
`CustomerStore` seam (satisfied by the account store) to read/persist the id.

**Dev keys.** Minted with a longer upstream expiry than the local reuse window
(gateway `expires_at` = now + 2d; astro-server re-mints after 1d) so rotation
always happens before the gateway key actually expires.

## Migration

- New nullable column `accounts.bifrost_customer_id` (schema.sql). No backfill —
  populated lazily on first key issuance per account.
- Deployment env: set `AI_GATEWAY_URL` (public base_url), `AI_GATEWAY_ADMIN_URL`
  (in-cluster governance endpoint), and `AI_GATEWAY_ADMIN_AUTH` (from the
  gateway admin-credentials Secret); remove `AI_GATEWAY_MASTER_KEY`. The Secret
  and endpoints are provisioned on the infra side.
