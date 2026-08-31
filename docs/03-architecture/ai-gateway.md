# AI Gateway

**Status:** Authoritative for `internal/aigateway` (key minting, budget,
metering). The Bifrost-otel section is a short overview, not a full
architecture pass — see [Bifrost-otel](#bifrost-otel) for why.
**Last verified:** 2026-08-31

## Summary

The AI Gateway lets a deployed agent call an LLM without ever holding a
provider credential. Astro operates a shared [Bifrost](https://github.com/maximhq/bifrost)
gateway per environment; astro-server mints a **virtual key** (VK) scoped to
the calling account and injects it into the agent's environment. The agent
calls the gateway's OpenAI-compatible endpoint with that key; Bifrost
resolves the real upstream credential (Bedrock), enforces the account's
spend ceiling in real time, and emits usage telemetry that becomes a
Metronome billing event.

This doc covers astro-server's side of that: `apps/astro-server/internal/aigateway/**`
and `apps/astro-server/handlers/ai_gateway_keys.go`. It also covers, more
briefly, `apps/bifrost-otel/**`, the metering leg that turns Bifrost's
telemetry into billing events — see [Bifrost-otel](#bifrost-otel) below for
why that section stays short.

The original design ([`06-plan/ai-gateway-astro-server.md`](../06-plan/ai-gateway-astro-server.md),
[`ai-gateway-dev-keys.md`](../06-plan/ai-gateway-dev-keys.md)) specced this
against **LiteLLM**. The team evaluated LiteLLM against Bifrost and Postman
Fabric before general availability and switched
([`07-feedback/fabric-vs-bifrost-gateway-evaluation.md`](../07-feedback/fabric-vs-bifrost-gateway-evaluation.md)):
feature-equivalent to LiteLLM, an order of magnitude lighter at idle. Treat
those two plan docs as historical design intent for the shape of the
integration (key-per-tenant, KMS-encrypted storage, rotation-worker idea),
not as a description of the current client. [`06-plan/ai-gateway-model-provider.md`](../06-plan/ai-gateway-model-provider.md)
is more current: the `provider: gateway` model entry and deploy-time model
selection it proposes has shipped (see [Spec surface](#spec-surface)).

## What this actually gates

**Key minting**, not model access. Every virtual key Bifrost issues here
grants the same thing: provider `bedrock`, `allowed_models: ["*"]`
(`internal/aigateway/client.go`'s `GenerateKey`). There is no per-model
allow-list, no per-agent scoping of which models a key can call — the model
whitelist the original LiteLLM plan described was never built, and the
`provider: gateway` deploy-time model **selector** (below) is a UI-level
menu, not an enforcement mechanism. Anything reachable through the shared
Bifrost `bedrock` provider config is reachable by every gateway key astro-server
mints.

**Budget enforcement**, in Bifrost, not astro-server. Each Astro account maps
to one Bifrost **customer**; every virtual key minted for that account
attaches to the customer and inherits its budget (no per-key budget). Bifrost
authorizes or rejects a request against that budget at call time; astro-server's
only job is keeping the ceiling correct (see [Budget sync](#budget-sync)).
Because Bifrost authorizes before the provider reports actual cost,
concurrent in-flight requests can overshoot the ceiling — this is a known,
accepted imprecision, not a bug.

Four kinds of key exist, all minted through the same `Client.GenerateKey`,
distinguished by name/metadata and by which store persists them:

| Kind | Minted when | Store | Lifetime | Revoked |
|---|---|---|---|---|
| Deployment key | `agent.astro_ai_gateway: true` or a `provider: gateway` model, at deploy apply | `deployment_ai_gateway` (`Store`) | No expiry; lives for the deployment's life, reused across redeploys | Undeploy, account purge |
| Dev key | `POST /api/v1/accounts/:account/ai-gateway-keys` (`astro dev`) | `account_ai_gateway_dev_keys` (`DevStore`) | Upstream TTL 48h; reused locally up to 24h before re-mint | Best-effort on re-mint of predecessor; account purge; otherwise ages out via upstream TTL |
| Judge key | First eval-dataset judge invocation for an account | `account_llm_judge_keys` (`JudgeStore`) | No expiry; one long-lived key per account | Account purge |
| — (none for astro-server admin calls) | — | — | — | — |

Every key request carries `AccountID`, which becomes both the Bifrost
`customer_id` (via the account's already-resolved customer) and the
attribution-bearing part of the VK name (`vkName` in `client.go`). This is
the same invariant the original LiteLLM plan called load-bearing for
attribution — it still holds, just against Bifrost's `customer_id` instead
of LiteLLM's `user_id`/`team_id`.

## Astro accounts vs Bifrost customers

`accounts.bifrost_customer_id` (`internal/account/store.go`'s
`Get/SetBifrostCustomerID`) links an Astro account to a Bifrost customer.
`Provisioner.ensureCustomer` (`provisioner.go`) is the only path that creates
one: idempotent, looks up the stored id first, and creates the Bifrost
customer with a card-less budget (`CardlessBudgetUSD = $10`) on first use.
Every key-minting path (`EnsureDeploymentKey`, `EnsureDevKey`, `EnsureJudgeKey`)
calls `ensureCustomer` before minting, so the very first gateway key an
account ever needs is also what provisions its Bifrost customer.

Creating the customer also, best-effort, syncs it onto the account's billing
customer as an ingest alias (`BillingAliaser.SyncBifrostAlias`) — see
[Connection to billing](#connection-to-billing).

astro-queen (the admin console) exposes `RecoverAccountBifrost`
(`internal/admingrpc/server.go`) as a manual repair action: it just calls
`EnsureCustomer` again, so it is safe to run on an account whose customer
link is missing or suspected stale.

## Budget sync

`internal/riverqueue/billing_gateway_budget.go`'s `BillingGatewayBudgetWorker`
keeps the Bifrost customer's monthly budget in step with the account's own
spend limit — it's the only control that can stop an uncollectible account
within the minutes it takes to run up a bill, since Bifrost enforces in real
time and the billing provider does not. Ceiling derivation (`ceilingUSD`):

- No Bifrost customer yet → no-op; a fresh customer already got the
  card-less default at creation.
- Exempt account → the account's ceiling as a floor, or the account's own
  limit if an operator set one above that floor.
- Account has its own spend limit set → that limit, clamped to the account's
  ceiling (`quota.SpendCeilingUSD`: an approved `spend_limit` quota request
  when it has one, else `billing.MaxSelfServeSpendUSD`). Clamping to the
  shared default instead would leave a granted account refused here after
  the billing provider already accepted the higher limit.
- No limit set → `CardedBudgetUSD` ($20) if the account has a payment method
  on file, else `CardlessBudgetUSD` ($10).

`BillingGatewayBudgetSweepWorker` re-applies the same derivation
periodically for the stalest accounts (`ListStaleGatewayBudgetAccounts`),
because nothing forces every writer that could move an input (a spend-limit
change, a card being added) to enqueue a re-derive — the sweep turns a missed
enqueue into a bounded delay instead of a permanent drift.

## Data flow: from spec to a metered LLM call

1. **Spec declares gateway use.** Either `agent.astro_ai_gateway: true`
   (deprecated but still honored) or a model entry with `provider: gateway`
   (see [Spec surface](#spec-surface)). `AstroSpec.UsesGateway()` is true if
   either is present.
2. **Admission.** The validator (`internal/deployment/validator.go`) rejects
   the spec if `UsesGateway()` is true but `AIGatewayEnabled` is false — set
   from `config.Deployment.AIGatewayURL != ""`. This is a hard admission
   gate: a spec that uses the gateway in an environment where it isn't
   configured never reaches deploy.
3. **Deploy apply.** `Deployer.Apply` (`internal/deployer/deployer.go`), when
   `ds.Agent.AIGateway` is true, calls
   `AIGatewayProvisioner.EnsureDeploymentKey` with the account, deployment,
   cluster, and agent identity. This mints (or reuses) the deployment's
   virtual key and returns the plaintext key plus the gateway's public base
   URL. A failure here fails the whole deploy — unlike Langfuse provisioning
   on the same code path, which only warns and continues. The difference is
   deliberate: losing tracing for one revision is tolerable, shipping an
   agent with no working model credential is not.
4. **Secret injection.** `spec_applier.go` writes `ASTRO_GATEWAY_URL` and
   `ASTRO_GATEWAY_API_KEY` into the agent's Kubernetes Secret when
   `ds.Agent.AIGateway && a.astroGatewayAPIKey != ""`. For a
   `provider: gateway` model entry with a `models:` option list,
   `template.go`'s `GatewayModelSelections` also generates a literal
   `MODEL_<SANITIZED_NAME>` env var carrying the deploy-time-selected model
   id (defaulting to the first option; the astro-client deploy form renders
   the picker as a normal select `Variable`, no client change needed).
5. **Agent calls the gateway.** The agent reads `ASTRO_GATEWAY_URL` /
   `ASTRO_GATEWAY_API_KEY` (and, for a `provider: gateway` model, `MODEL_<NAME>`
   for which model to ask for) and calls Bifrost's OpenAI-compatible
   `/v1/chat/completions` endpoint directly with `Authorization: Bearer <key>`.
   Astro-server is not in this request path at all — `InvocationClient`
   (`invocation.go`) is a second, separate client used only by astro-server
   itself (the eval judge and evaluator, see below), not by deployed agents.
6. **Bifrost authorizes and serves the request**, checking the virtual key
   against its customer's live budget, resolving the real Bedrock credential,
   and emitting an OTLP trace span carrying token counts, resolved model,
   cost, and the virtual-key/customer identity.
7. **Metering.** `bifrost-otel` consumes that trace and turns the billable
   span into a Metronome usage event keyed on the account id. See
   [Bifrost-otel](#bifrost-otel) and [Connection to billing](#connection-to-billing).

Dev keys and the judge key skip steps 1–4: they're minted directly by an
HTTP endpoint (dev keys, `handlers/ai_gateway_keys.go`) or by first use
(judge key, `evaljudge`), not by the deploy pipeline. Both still go through
the same `ensureCustomer` → `GenerateKey` → encrypt → persist shape.

## Storage and encryption

Every stored key uses the same envelope-encryption shape
(`internal/envelope`): `vault.Encryptor` produces ciphertext, an
encrypted data key, and a nonce; only those three values are persisted,
never plaintext. `Store`, `DevStore`, and `JudgeStore` are otherwise thin —
each is a `database/sql` wrapper over one table
(`deployment_ai_gateway`, `account_ai_gateway_dev_keys`,
`account_llm_judge_keys`) with `Get`/`Save`/`Delete`/`ListByAccount`-shaped
methods. A KMS-based orphan-on-failure pattern is consistent across all
three mint paths (`EnsureDeploymentKey`, `EnsureDevKey`, `EnsureJudgeKey`):
if encrypting or persisting the freshly-minted key fails, the code
best-effort deletes the just-minted upstream key rather than leaving an
unreferenced VK live in Bifrost.

## Revocation

Three account-scoped sweeps exist, called from different lifecycle events:

- `Provisioner.RevokeAccount` — every `deployment_ai_gateway` row under the
  account, deleted upstream then locally. Called on account purge
  (`internal/riverqueue/purge_accounts.go`).
- `Provisioner.RevokeAccountDevKeys` / `RevokeAccountJudgeKeys` — same shape
  for dev keys and the judge key. Called on account purge, and also from
  `billing_suspend.go` when an account is suspended for non-payment. Deployment
  keys are deliberately left alone on suspend: suspend only scales the
  account's workloads to zero, it doesn't undeploy them, and the key's
  plaintext already lives in the tenant Secret that a resume re-applies
  rather than re-mints. Dev and judge keys are revoked because both are
  re-minted on demand, so there's no equivalent resume path that needs them
  intact.
- `Provisioner.RevokeDeploymentKey` — single-deployment revoke, called on
  undeploy.

All are best-effort at the per-key level: one key's upstream delete failing
doesn't block the rest of the sweep, and a `JudgeKey` (the one case that
matters most, since it has no expiry) preserves its local row on upstream
failure so a retry doesn't lose the only record of the key ID.

## Spec surface

Two ways to opt in, one of them deprecated:

- **`agent.astro_ai_gateway: true`** (`packages/astro-spec/spec.go`). Boolean,
  no model selection — the agent hard-codes which model it asks Bifrost for.
  Still parsed and honored; deprecated in favor of the model-provider form.
- **A model entry with `provider: gateway`** (`GatewayProviderName` in
  `packages/astro-spec/provider.go`). Declares one or more selectable model
  ids (`models: [claude-sonnet-4-6, gpt-4o, ...]`); the deploy form renders
  them as a `display-as: select` Variable, and the choice is injected as
  `MODEL_<SANITIZED_ENTRY_NAME>`. `ASTRO_GATEWAY_URL`/`_API_KEY` are the same
  shared pair regardless of which model is picked — the selector only picks
  which model id the agent asks Bifrost for, not a different credential.

The two are mutually exclusive on the same spec (parser rejects both set at
once). Both derive the same `DeploymentAgent.AIGateway` boolean
(`AstroSpec.UsesGateway()`) that drives admission and key minting — there is
no separate enablement path for the newer form.

As of this doc, the model list in a `provider: gateway` entry is
**selection-only**: it changes the default env value and the deploy-form
picker, not what the minted Bifrost key is allowed to call (see
[What this actually gates](#what-this-actually-gates)).

## Connection to billing

AI Gateway spend feeds the **same Metronome pipeline** compute usage does,
not a separate one — this doc doesn't re-derive that pipeline; see
[`03-architecture/billing-architecture.md`](billing-architecture.md)
(Pipeline 2, "LLM tokens") for the full mechanics, wire format, and the
verified live billable-metric behavior. In short, from this package's side:

- `billing.AliasSyncer.SyncBifrostAlias` links an account's Bifrost customer
  id onto its Metronome customer as a second ingest alias, so gateway usage
  and compute usage both attribute to the same Metronome customer with no
  mapping table. Called best-effort every time a Bifrost customer is
  created — a failure here never blocks key minting.
- `BillingGatewayBudgetWorker`/`Sweep` (above) is the gating half: it keeps
  Bifrost's own real-time enforcement in step with the account's billing
  spend limit, independent of Metronome's usage-based invoicing.
- Astro-server never sends gateway usage to Metronome itself — that's
  entirely `bifrost-otel`'s job, downstream of Bifrost's own telemetry, not
  astro-server's.

## Config and wiring

Three env vars, all on `config.Deployment` (`internal/config/config.go`),
empty is the disable signal (no separate enabled flag):

| Var | Purpose |
|---|---|
| `AI_GATEWAY_URL` | Public gateway base URL, written into tenant Secrets as `ASTRO_GATEWAY_URL` and used as `InvocationClient`'s target |
| `AI_GATEWAY_ADMIN_URL` | In-cluster Bifrost governance API; falls back to `AI_GATEWAY_URL` if unset (single-URL/local-dev) |
| `AI_GATEWAY_ADMIN_AUTH` | Full `Authorization` header value for the governance API (HTTP Basic, ESO-delivered) |

`main.go` constructs one `Provisioner` at startup (nil when `AI_GATEWAY_URL`
is empty) and shares it across the HTTP handler, the deployer
(`internal/riverqueue/workers.go` wires the same shapes into River workers),
`evaljudge`, `evaluator`, and `billing_suspend`'s revoke calls. There's no
per-caller provisioner — one client, one set of credentials, reused
everywhere.

## Bifrost-otel

`apps/bifrost-otel` is an OpenTelemetry Collector Builder distribution: an
OTLP receiver plus one custom exporter
(`internal/exporter/bifrostotel`) that turns Bifrost's per-request GenAI
trace spans into Metronome usage events. It picks the final successful
attempt of each request (so a retried call bills once, not per attempt),
maps span attributes to a Metronome event keyed on the Bifrost customer id,
and posts batches to Metronome's `/v1/ingest` with the Bifrost request id as
the idempotency key.

This doc keeps that description short deliberately, not because the
component is thin. Two things already cover it in depth and this doc
doesn't repeat them:

- [`billing-architecture.md`](billing-architecture.md)'s "Pipeline 2: LLM
  tokens" section is the canonical description of the wire format, the
  retry-dedupe correctness argument, and a live-verified note that the
  Metronome billable metric actually sums `cost_usd`, not the raw token
  counts the exporter also carries.
- `modules/astro-infra`'s own `docs/architecture/16-llm-usage-metering.md`
  is a full design writeup with production data (a real over-billing
  incident from grouping by trace instead of by request, since fixed). It
  isn't part of this repo's `docs/` tree or area map (astro-infra keeps its
  own docs, per this repo's top-level `CLAUDE.md`), but it's the deeper
  reference if you need collector-internals detail beyond what's here.

**Git history caveat.** `apps/bifrost-otel` shows one commit in this repo
(`309866ac0`, "build and deploy the collector from the monorepo") as of this
writing. That commit message is explicit that the code is not new: it moved
the already-running collector's source into this monorepo so CI could build
and deploy it — "the exporter is byte-identical apart from gosec
suppressions." Its test suite (`exporter_test.go`, ~200 lines) and the two
docs above are what let this doc describe it beyond a stub despite the thin
in-repo history; treat any *change* to `apps/bifrost-otel` as touching a
component with real production behavior, not as touching a fresh scaffold.

## Verify

```sh
cd apps/astro-server && go test ./internal/aigateway/... && go test ./handlers/... -run TestIssueAIGatewayDevKey
```

43 test functions across `internal/aigateway`'s `_test.go` files as of this
writing (client, deployment key, dev key, judge key/store, invocation), plus
one handler test for the nil-dependency 503 path. `apps/bifrost-otel` has its
own suite: `cd apps/bifrost-otel && make test`.
