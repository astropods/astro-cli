# AI Gateway — dev keys for local agents

**Status:** Draft
**Date:** 2026-06-03
**Owner:** saswat@postman.com
**Companion to:** [ai-gateway-astro-server.md](ai-gateway-astro-server.md)

## Why

`astro dev` runs an agent container on the developer's laptop — no IRSA, no
in-cluster Secrets, no kubelet to mount the deploy-time `astro-ai-gateway`
Secret. If the agent declares `provider: astro-gateway`, the local container
needs the same `ASTRO_GATEWAY_*` env vars the deployer would inject in prod,
but populated from a key the *developer* is authorized to use — not the
LiteLLM master key, not the durable per-tenant key minted at deploy.

## Design

### Endpoint

One new endpoint on astro-server, scoped to an account the caller is a
member of:

```
POST /api/v1/accounts/:account/ai-gateway-keys
```

No body. The gateway is one service per env; a dev key is scoped to the
account, not to any specific model entry. The CLI maps the returned
key+URL onto whichever `provider: astro-gateway` entries its local spec
declares.

Response:

```json
{
  "key_id":     "tok-abc",
  "api_key":    "sk-astro-...",
  "base_url":   "https://aig.astropod.ai",
  "expires_at": "2026-06-04T08:00:00Z"
}
```

The CLI uses `spec.ModelCredentialKeysForProvider` (already exported from
`packages/astro-spec`) to derive the resolver-correct env-var names for
its local spec's `astro-gateway` entries, then assigns the returned
`api_key` to each `*_API_KEY` slot and `base_url` to each `*_BASE_URL`
slot. The naming logic lives in the spec package — same source of truth
as the deployer-side injection.

**No revoke endpoint.** Dev key lifecycle is managed entirely by the
LiteLLM-side TTL set at issuance time. Keys expire automatically; the
CLI has no cleanup responsibility — every `astro dev` invocation fetches
a fresh key, and abandoned keys age out on their own.

### Key minting rules

- `user_id = account.ID`, `team_id = account.ID` — same load-bearing
  attribution invariant as the deploy-time path. OpenMeter rolls dev and
  prod spend up to the same account-id subject, so chargeback stays
  correct.
- `metadata = {kind: "dev", actor_user_id, machine_id}` — `kind` lets
  audit queries separate dev keys from deploy keys; `actor_user_id` makes
  the inevitable "whose key is this" question answerable; `machine_id` is
  a CLI-supplied hash of `hostname + os.uid` for further forensics
  granularity.
- `duration: "8h"` — LiteLLM enforces upstream expiry. The CLI's
  Ctrl-C-driven DELETE is the fast path; the 8h cap is the safety net
  for sessions that crash without cleanup.
- **Not** persisted in `account_ai_gateway`. That table holds the
  durable tenant key the deployer mints; dev keys live only in LiteLLM's
  own DB plus a transient CLI state file.

### Base URL

The gateway is reachable over the public internet on a TLS endpoint
gated by the API key — same hostname every caller uses, no in-cluster
vs out-of-cluster split. `AI_GATEWAY_URL` already carries this value,
so the dev-keys response returns it as `BASE_URL` without any new
config field. When `AI_GATEWAY_URL` is unset (gateway not configured
in this env), the dev-keys endpoint returns 503.

### Spec helper

The existing `spec.DeploymentModelCredentialKeys(*AstroDeploymentSpec,
provider)` walks `ds.Models` to find provider matches. The dev-keys
endpoint doesn't have a deployment spec — it has a list of entry names.
A new helper:

```go
func ModelCredentialKeysForProvider(entryNames []string, provider string) map[string]string
```

reuses the §8.1 logic (bare vs qualified, sanitization) but operates on a
caller-supplied list of names. Both helpers stay in the spec package so
the resolver remains the single source of truth.

### CLI changes (`astro dev`)

- On `astro project start`:
  1. Check `astroSpec.Agent.AIGateway`.
  2. If true, call `POST /accounts/:account/ai-gateway-keys`.
  3. Set `ASTRO_GATEWAY_URL` to the response `base_url` and
     `ASTRO_GATEWAY_API_KEY` to the response `api_key`. Merge into the env
     map `buildLocalAgentEnv` passes to the local agent container.

No cleanup on stop — the upstream TTL handles it.

## Security

- Caller auth is the standard user bearer token — no extra credential
  type. Account membership is enforced before any LiteLLM call.
- The minted key is bearer-only; if it leaks, the blast radius is the
  account's gateway spend until the 8h TTL or a manual revoke. WAF
  rate-limit on the public ALB is the coarse second line; per-key
  budget alerts in OpenMeter are the third.
- No new persistence on astro-server. Dev keys are not stored in
  Postgres at all — the state-of-truth is LiteLLM's own DB. This
  intentionally keeps the dev workflow lightweight; we don't want a
  laptop reboot to leave orphaned rows here.

## Out of scope

- **Long-lived personal keys** for offline dev. If devs want to work on
  a flight, a separate "personal access key" flow with explicit user
  storage is a follow-up.
- **Per-developer billing attribution.** Keys mint with `user_id =
  account.id` so OpenMeter aggregates correctly; per-user breakdown
  lives only in `metadata.actor_user_id` for audit, not as a separate
  billing dimension. Splitting "dev pool" from "prod pool" in OpenMeter
  is a follow-up if a tenant's dev usage starts threatening prod budget.
- **Quota / rate-limit per dev key.** v1 uses the same per-account
  budget as deploy-time keys. Per-dev-key quota would prevent a runaway
  test script from eating the tenant's gateway spend, but it adds a
  config surface that's premature.
