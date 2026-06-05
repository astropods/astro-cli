# AI Gateway — astro-server integration

## Summary

Astro now mints, stores, and injects per-deployment LiteLLM virtual keys
against the AI Gateway. A spec opts in via `agent.astro_ai_gateway: true` and
gets a `ASTRO_GATEWAY_URL` + `ASTRO_GATEWAY_API_KEY` pair in the agent
container — no upstream provider credential ever touches the spec, the
user, or the build pipeline. The gateway routes to whichever model the
agent code picks at call time; model choice is not declared in the spec.

The gateway itself (LiteLLM, Postgres/Redis, WAF, helm) lives in
`modules/astro-infra`. This change is the astro-server side:
provisioning, key lifecycle, deploy-time injection, and removal of the
legacy `anthropic-managed` provider that it supersedes.

Design context lives in
[docs/plans/ai-gateway-astro-server.md](../plans/ai-gateway-astro-server.md).
The local-dev key flow is documented separately in
[docs/plans/ai-gateway-dev-keys.md](../plans/ai-gateway-dev-keys.md).

## Design

### Spec surface — one boolean opt-in

```yaml
agent:
  image: my-agent:latest
  astro_ai_gateway: true
```

That's the whole spec change. No `provider:` entries, no model whitelist
in the spec, no per-entry credentials. The runtime injects:

```
ASTRO_GATEWAY_URL=<gateway endpoint>
ASTRO_GATEWAY_API_KEY=<per-deployment virtual key>
```

Agent code picks the model at call time:

```python
client = OpenAI(
    api_key=os.environ["ASTRO_GATEWAY_API_KEY"],
    base_url=os.environ["ASTRO_GATEWAY_URL"],
)
client.chat.completions.create(model="claude-sonnet-4-6", ...)
```

The gateway is *access to a service*, not a model provider. One URL and
key reach every model the gateway routes — per-model fanout in the spec
would add credential namespace noise without buying anything.

### Per-deployment virtual keys

New `internal/aigateway` package mirrors `internal/langfuse`:

- `Client` — thin LiteLLM admin HTTP client. 404 on delete is treated as
  success for retry safety.
- `Store` — `deployment_ai_gateway` table holding the KMS-envelope-
  encrypted ciphertext keyed by `deployment_id`. `account_id` is
  denormalized so account-purge can sweep without traversing
  `deployments`.
- `Provisioner` — `EnsureDeploymentKey` / `RevokeDeploymentKey` /
  `RevokeAccount`. Idempotent at every layer.

**Load-bearing invariant.** The LiteLLM key's `user_id` and `team_id`
MUST be the Astro account-id. OpenMeter's chargeback rolls up
`metadata.user_api_key_user_id` — any drift silently corrupts the spend
ledger. Deployment scope lives in `metadata.tags`
(`deployment:<id>`, `agent:<name>`, `version:<v>`) for filtering in
LiteLLM's admin views.

Per-tenant Langfuse credentials are deliberately *not* embedded in key
metadata; that would land plaintext `sk-lf-*` values in LiteLLM's admin
DB, widening the blast radius if the gateway were compromised.
Gateway-side observability stays on the collector path — agents emit
OTel to the astro-collector, which routes to the per-account Langfuse
project using KMS-encrypted credentials in `account_langfuse`.

### Lifecycle

| Event | Action |
|---|---|
| First deploy with `agent.astro_ai_gateway: true` | `EnsureDeploymentKey` mints, KMS-encrypts, persists |
| Redeploy of same deployment | `EnsureDeploymentKey` decrypts existing row and returns the same key — Secret content is stable across rollouts |
| Undeploy (`Deployer.Teardown`) | Revokes upstream + deletes row (best-effort; row stays if upstream is unreachable, account-purge retries) |
| Account purge | `RevokeAccount` iterates every deployment under the account, revokes each upstream, deletes rows |

No auto-rotation. Mint-at-deploy / decrypt-at-redeploy / revoke-at-
undeploy is sufficient for v1; deploys are infrequent enough that
explicit rotation can land later as a deployment-template API rather
than a background hygiene policy.

### Deploy-time injection

`internal/deployer/deployer.go` calls `EnsureDeploymentKey` when
`ds.Agent.AIGateway` is true and threads the result into
`internal/k8s/spec_applier.go`, which writes the URL + key pair into
the agent's K8s Secret. **Fail-hard semantics** — a deploy that opts
into the gateway fails outright when the gateway isn't reachable,
rather than shipping a pod that 401s on its first model call. The
previous revision keeps running, so live traffic is unaffected.

### Validator gate

`deployment.NewValidatorWithOptions(ValidatorOptions{AIGatewayEnabled: …})`
rejects `agent.astro_ai_gateway: true` at admission when the gateway isn't
configured in the env. Both validator call sites
(`handlers/agents.go` for CLI pushes, `internal/githubbuild/pipeline.go`
for GitHub builds) thread `cfg.Deployment.AIGatewayURL != ""` through.
The deployer rejects again at deploy time as defense in depth.

No model whitelist in the validator — the gateway validates models at
call time and the supported set drifts faster than the spec package.

### Local dev — singular pair, cached short-lived key

New endpoint `POST /api/v1/accounts/:account/ai-gateway-keys` mints
short-lived (8h TTL) keys for `astro dev`. Per-(account, user) row in
`account_ai_gateway_dev_keys`; non-expired rows are decrypted and
returned across invocations, so successive `ast dev` runs don't burn a
fresh upstream key each time. On expiry the predecessor is
best-effort revoked upstream and replaced.

Same load-bearing `user_id = team_id = account_id` invariant as the
deploy-time path, so dev and prod spend roll up to the same OpenMeter
subject. CLI calls this on startup when `astroSpec.Agent.AIGateway` is
true; the response flows into `ASTRO_GATEWAY_API_KEY` /
`ASTRO_GATEWAY_URL` in the local container — identical to what the
deployer would inject in prod.

No CLI-side cleanup. The server-side TTL is the only lifecycle
mechanism.

### `anthropic-managed` removal

Deleted in the same PR. The new `agent.astro_ai_gateway` opt-in supersedes
`provider: anthropic-managed` along every axis — per-tenant attribution,
multi-provider routing, gateway-level guardrails. No live deployment
specs referenced `anthropic-managed`, so the removal is a clean delete
rather than a soak-gated migration.

### Config

Two flat fields on `DeploymentConfig`:

```go
AIGatewayURL       string // AI_GATEWAY_URL — LiteLLM public endpoint
AIGatewayMasterKey string // AI_GATEWAY_MASTER_KEY — ESO-delivered
```

The gateway is publicly reachable over TLS; auth is the gate, not the
network. Same URL serves astro-server admin calls, deploy-time pod
injection, and local-dev container calls.

## Migration

**Agent authors.** Set `agent.astro_ai_gateway: true` in `astropods.yml` and
read `ASTRO_GATEWAY_URL` / `ASTRO_GATEWAY_API_KEY` in agent code. No
other spec changes; existing `models:` / `knowledge:` / `integrations:`
entries are unaffected.

**Operators.** Unset `MANAGED_ANTHROPIC_API_KEY` (now ignored). Set
`AI_GATEWAY_URL` + `AI_GATEWAY_MASTER_KEY` (already wired in the
preview helm values template). The `astro-server-ai-gateway` Secret
carries the master key via ESO; no manual rotation needed.

**BYOC tenants.** Unchanged in v1 — preview is single-cluster only.
Tenants reach the same public gateway URL as everyone else.
