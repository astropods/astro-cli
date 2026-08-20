# AI Gateway — astro-server integration

**Status:** Draft
**Date:** 2026-06-03
**Owner:** saswat@postman.com
**Companion to:** [ai-gateway.md](../../modules/astro-infra/docs/plans/ai-gateway.md)

> **Design update (post-implementation).** The original draft modeled the
> gateway as a builtin `provider: astro-gateway` in `models.*` with per-entry
> credential fanout (`ASTRO_GATEWAY_WORKHORSE_API_KEY` etc.). After
> implementing it we simplified: the gateway is a *capability*, not a
> model provider. The spec is now `agent.astro_ai_gateway: true` (boolean), and
> the runtime injects a singular pair `ASTRO_GATEWAY_URL` + `ASTRO_GATEWAY_API_KEY`.
> Model selection happens at call time in agent code; no model whitelist,
> no per-entry naming. The internal/aigateway lifecycle plumbing
> (provisioner, store, rotation, purge, dev-key endpoint) stays the same —
> it's the spec surface that simplified. See changelog for the migration.

## Why

The preview gateway (`aig.astropod.ai`) is live. Phase 1 infra is applied; the LiteLLM workload is reachable on the public hostname with the master key. What's missing is the bit that makes a *tenant* able to use it: when an agent declares `agent.astro_ai_gateway: true` in its `astropods.yml`, the agent pod needs `ASTRO_GATEWAY_URL` and `ASTRO_GATEWAY_API_KEY` set to the gateway endpoint and a **per-tenant** virtual key, with no upstream credential ever touching the spec, the user, or the build pipeline.

This plan covers the astro-server work for that path — declaring the provider, minting and storing per-account virtual keys, injecting them into the agent's K8s Secret at deploy time, and rotating them. It is scoped to **preview-only** for the first cut; prod is a copy once preview holds for two weeks.

> **Load-bearing invariant.** The LiteLLM virtual key minted for an account is always generated with `user_id = <account-id>` and `team_id = <account-id>` — the `accounts.id` primary key, nothing else. OpenMeter rolls up per-tenant spend on that subject; any drift (email, slug, user UUID, account name) silently corrupts the chargeback ledger and there is no downstream code that can repair it. See §2 for the assertion + test that pins this.

## What's already in place

Worth being explicit because the design leans on these — none of this needs to be built:

- **`Managed` provider mechanism.** `packages/astro-spec/provider.go` already encodes a `Managed bool` on `BuiltinProvider`. The precedent is `anthropic-managed`: cloud provider, no user-supplied credentials, server injects at deploy time. Spec validator already short-circuits credential checks for `Managed` providers (`internal/deployment/validator_test.go:361`). **`anthropic-managed` itself is going away as part of this work** — see §6 — but the mechanism it pioneered is exactly the shape `astro-gateway` needs.
- **Server-side credential injection at apply time.** `internal/k8s/spec_applier.go:90` already pulls `ManagedAnthropicAPIKey` from `ApplierConfig` and writes it into the agent's Secret as `ANTHROPIC_API_KEY`. The "astro-gateway" provider plugs into the exact same site, then that block is rewritten to drop the now-obsolete `ManagedAnthropicAPIKey` path entirely.
- **Deploy-time hook for per-account upstream provisioning.** `internal/deployer/deployer.go:102` already calls `LangfuseProvisioner.EnsureProject` per-account on every deploy and threads the result into the apply. Same hook is where the AI Gateway virtual-key ensure call lands.
- **KMS envelope encryption pattern.** `internal/knowledgestore/credentials.go` encrypts a per-account credential with the deployment KMS key and stores the ciphertext, wrapped data key, and nonce in Postgres; that pattern is the direct template for storing the LiteLLM virtual key.
- **River-queue scheduled workers.** `internal/riverqueue/periodic.go` is the scheduling surface; `purge_accounts.go` is the precedent for "do an upstream cleanup when an account dies."

## What we're adding

Four pieces, in dependency order:

### 1. Declare the `astro-gateway` builtin provider — `packages/astro-spec`

Add one entry to `builtinProviders` in `packages/astro-spec/provider.go`:

```go
{
    Name: "astro-gateway", Section: "models", Cloud: true, Managed: true,
    Credentials: []CredentialSuffix{
        {Suffix: "API_KEY",  Description: "Astro AI Gateway key (provided by platform)"},
        {Suffix: "BASE_URL", Description: "Astro AI Gateway endpoint (provided by platform)"},
    },
},
```

`Managed: true` is load-bearing — the validator already skips "you must supply credentials" for managed providers, so a spec author writes:

```yaml
models:
  workhorse:
    provider: astro-gateway
    model: claude-sonnet-4-6
  fast:
    provider: astro-gateway
    model: claude-haiku-4-5
```

and nothing else. No `inputs:`, no API key field, no env-var wiring on the user side. The credential suffixes go through the existing env-resolver, which derives qualified env-var names per the rule already in `packages/astro-spec/envresolver*.go` (see `envresolver_test.go:142-184` for the full behaviour):

| Spec | Generated env vars |
|---|---|
| Single entry name matches provider: `models.astro_gateway.provider: astro-gateway` (or `models.astro-gateway...` — both sanitize to `astro_gateway`) | `ASTRO_GATEWAY_API_KEY`, `ASTRO_GATEWAY_BASE_URL` (bare) |
| Single entry with custom name: `models.workhorse.provider: astro-gateway` | `ASTRO_GATEWAY_WORKHORSE_API_KEY`, `ASTRO_GATEWAY_WORKHORSE_BASE_URL` (qualified) |
| Multiple entries: `workhorse` + `fast` both `provider: astro-gateway` | `ASTRO_GATEWAY_WORKHORSE_*`, `ASTRO_GATEWAY_FAST_*` (qualified only — no bare) |

**Why this is right, not the hardcoded-SDK-name approach.** Declaring the credentials properly buys three things at once:

1. **Multiple gateway-routed models per agent** each get their own clearly-addressable env vars. An agent can declare a `workhorse` for reasoning and a `fast` for tool-use loops, and the agent code reads `ASTRO_GATEWAY_WORKHORSE_*` vs `ASTRO_GATEWAY_FAST_*` — no collision, no "which model owns `OPENAI_API_KEY`?" question.
2. **Composition with non-gateway providers in the same spec.** A spec can mix `provider: astro-gateway` (managed, via gateway) with `provider: openai` (BYOK, direct) and `provider: anthropic` (BYOK, direct) — each gets its own credential namespace via the resolver. No env-var collision with bare `OPENAI_API_KEY` / `ANTHROPIC_API_KEY`.
3. **One small resolver change, no per-provider special-case.** The existing suffix → `<PROVIDER>_[<NAME>_]<SUFFIX>` rule handles `astro-gateway` the same way it handles every other cloud provider — *once we fix the resolver to emit env-var names for `Managed: true` providers at all*. Today `packages/astro-spec/envresolver.go:213` `continue`s past managed providers entirely (`managed providers don't generate user-facing credentials`) — which is why `anthropic-managed` had to hardcode its env var (`ANTHROPIC_API_KEY`) in spec_applier. That `continue` has to go: the right meaning of `Managed: true` is "the *value* is server-supplied", not "no env-var name is emitted". After the change, the resolver produces credential names for managed providers, the validator still skips the "user must supply value" check for them (orthogonal concern, already done in a different code path), and the applier fills the values in. `anthropic-managed`'s bespoke hardcoded path disappears as part of §6.

The spec author still gets a one-line declaration; the agent author writes the SDK call once, explicitly:

```python
client = OpenAI(
    api_key=os.environ["ASTRO_GATEWAY_WORKHORSE_API_KEY"],
    base_url=os.environ["ASTRO_GATEWAY_WORKHORSE_BASE_URL"],
)
```

That's marginally more verbose than `OpenAI()` reading `OPENAI_*` out-of-box, but eliminates the magic and is the same explicitness already required for `ANTHROPIC_SONNET_API_KEY` and friends today.

**Why one builtin entry, not two (`astro-gateway` + `astro-managed`):** the gateway is *always* platform-managed — there's no BYOK form. Keeping it as a single `astro-gateway` name avoids the redundant `-managed` suffix `anthropic-managed` carried (and which we're deleting in §6 anyway).

**Validation extension.** The model whitelist (`claude-sonnet-4-6`, `claude-haiku-4-5`, `titan-embed-text-v2` — the v1 Bedrock set from `ai-gateway.md`) is enforced *server-side* in the validator, not in the spec parser, because the supported model list will drift faster than the spec package versions. New package: nothing. New validation check: `deployment.Validator` rejects `provider: astro-gateway` with a model not in the env's gateway model list. The list comes from the same source the LiteLLM chart uses (`helm/values/common/ai-gateway.yaml.tpl` — eventually surfaced through config; for v1 a constant in `internal/aigateway/models.go` is fine).

### 2. `internal/aigateway/` — the provisioner package

Mirrors `internal/langfuse/`. Three files:

**`client.go`** — HTTP client over LiteLLM admin API, base URL = the gateway's public hostname (`https://aig.astropod.ai`). Same endpoint every caller (astro-server admin, deploy-time tenant pods, local dev containers) uses — auth is the gate, not the network.

```go
type Client struct { baseURL, masterKey string; http *http.Client }

func (c *Client) GenerateKey(ctx, KeyRequest) (KeyResponse, error)  // POST /key/generate
func (c *Client) DeleteKey(ctx, keyID string) error                 // POST /key/delete
func (c *Client) UpdateKey(ctx, keyID string, KeyUpdate) error      // POST /key/update — for metadata edits / budget bumps
func (c *Client) KeyInfo(ctx,   keyID string) (KeyInfo, error)      // GET  /key/info — diagnostic only
```

**`KeyRequest.user_id` MUST be the astro account-id** — `accountID` from the `accounts` table primary key, nothing else. The same value goes into `team_id`. `metadata` carries `customer_id`, `cluster_id`, `deployment_count`. (No `environment` field — each env runs its own gateway, so every key issued by a given gateway is implicitly scoped to that env; carrying the value would be redundant noise on every event.)

This is the single most load-bearing invariant in the integration:

- OpenMeter's billing rollup keys off `user_id` (`subject` on the CloudEvent). If `user_id` is anything other than the account-id, per-tenant spend rolls up to the wrong subject and chargeback breaks silently. Worse: any change to the rule (e.g. "use email" or "use user UUID") retroactively invalidates the historical ledger because OpenMeter dedupes/aggregates against whatever string was on the event at write time.
- The plan ([ai-gateway.md §Billing pipeline](ai-gateway.md)) calls out that LiteLLM's stock openmeter callback resolves subject from `kwargs["user"]` first, then falls back to the key's `metadata["user_api_key_user_id"]` — which is set at `/key/generate` time from this field. Astro-server is the only writer of that value; if we get it wrong, no downstream code can fix it.
- Keys minted without a `user_id` fail OpenMeter ingest loudly ("user is required"). That's a deliberate loudness, not a bug. The client asserts the field is non-empty before serializing — missing-`user_id` is a programmer error, not a runtime one.

**Concretely** — in `provisioner.go`, the only call site is:

```go
func (p *Provisioner) EnsureTenantKey(ctx, accountID, accountName, customerID, clusterID string) (...) {
    // accountID flows directly into both user_id AND team_id. Do not derive
    // either from accountName, email, slug, or any other identifier — those
    // can change; the account-id cannot.
    req := KeyRequest{
        UserID: accountID,
        TeamID: accountID,
        Metadata: map[string]string{
            "customer_id":  customerID,
            "cluster_id":   clusterID,
            "account_name": accountName, // observability only — NOT used for attribution
        },
    }
    // ...
}
```

Unit test in `provisioner_test.go` pins this: a fake LiteLLM admin server records the inbound `/key/generate` payload, the test asserts `payload.user_id == accountID` and `payload.team_id == accountID`. Any future change to the call signature has to update the test; a refactor that quietly swaps in a different identifier breaks CI.

**`provisioner.go`** —

```go
type Provisioner struct {
    Client    *Client
    Store     *Store
    KMSClient envelope.KMSClient
    KMSKeyARN string
    BaseURL   string  // PUBLIC hostname — what we write into the tenant Secret
    Env       string  // "preview" / "prod" — goes into key metadata
}

func (p *Provisioner) EnsureTenantKey(ctx, accountID, accountName, customerID, clusterID string)
    (apiKey string, baseURL string, err error)
```

Idempotent: `Store.Get(accountID)` first; if present and `RotatesAt > now`, decrypt and return. Otherwise `GenerateKey`, encrypt the returned plaintext under the gateway KMS key, persist (`KeyID`, `EncryptedAPIKey`, `IssuedAt`, `RotatesAt = now + 30d`), return plaintext + public base URL.

`RevokeTenantKey(ctx, accountID)` — used at account purge. Calls `DeleteKey` for both current and previous keyIDs, then deletes the store row. Same place `purge_accounts.go` already does Langfuse project deletion.

**`store.go`** —

```sql
CREATE TABLE account_ai_gateway (
    account_id           TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    key_id               TEXT NOT NULL,           -- LiteLLM keyID, used for /key/delete
    encrypted_api_key    BYTEA NOT NULL,          -- envelope-encrypted plaintext
    key_id_prev          TEXT,                    -- non-null during rotation overlap
    encrypted_api_key_prev BYTEA,
    issued_at            TIMESTAMPTZ NOT NULL,
    rotates_at           TIMESTAMPTZ NOT NULL,
    prev_expires_at      TIMESTAMPTZ,             -- non-null during rotation overlap; when reached, prev is purged
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Migration goes alongside the existing migration set. Shape mirrors `account_langfuse`.

### 3. Deploy-time injection — `internal/deployer/deployer.go` + `internal/k8s/spec_applier.go`

Two small additions to the existing flow.

**In the deployer**, immediately after the Langfuse ensure block (around line 113):

```go
var aigwAPIKey, aigwBaseURL string
if d.AIGatewayProvisioner != nil && deploymentReferencesAstroGateway(ds) {
    apiKey, baseURL, err := d.AIGatewayProvisioner.EnsureTenantKey(
        ctx, acct.ID, acct.Name, acct.CustomerID, dep.EffectiveClusterID(),
    )
    if err != nil {
        // Fail-hard, per ai-gateway.md "Decisions" — the attribution guarantee
        // matters more than per-deploy success. The deploy fails; the operator
        // sees a clear gateway-related error and retries once the gateway is back.
        return nil, fmt.Errorf("ensure ai-gateway key for account %s: %w", acct.Name, err)
    }
    aigwAPIKey, aigwBaseURL = apiKey, baseURL
}
```

`deploymentReferencesAstroGateway(ds)` walks `ds.Models` and returns true if any entry has `Provider == "astro-gateway"`. Cheap, deploy-time-only.

**Why fail-hard instead of warn-and-continue (like Langfuse).** Langfuse is observability — a deploy that loses tracing for one revision is annoying but the agent still serves traffic. The Astro provider key is the agent's *primary credential to call any model at all*. Shipping a deploy without it is shipping a broken agent. Better to fail the deploy and surface "gateway unreachable" than to silently produce a pod that 401s on its first model call.

**Pass through `ApplierConfig`** (`internal/k8s/applier.go`):

```go
type ApplierConfig struct {
    // ...existing fields...
    ManagedAnthropicAPIKey string
    AstroGatewayAPIKey     string  // per-deploy, per-account; empty if no astro-gateway provider in spec
    AstroGatewayBaseURL    string  // per-env public hostname
}
```

**In `spec_applier.go`**, replace the existing managed-credential injection block at line 90 (the `ManagedAnthropicAPIKey` path goes away — see §6). The applier doesn't know the env-var names directly — it asks the resolver, which already computed them from `astro-gateway`'s `Credentials` declaration in §1:

```go
if a.astroGatewayAPIKey != "" {
    for modelName, model := range ds.Models {
        if model.Provider != "astro-gateway" {
            continue
        }
        // Resolver-computed env-var names for this entry's credentials.
        // For `models.workhorse.provider: astro-gateway` this returns
        // ["ASTRO_GATEWAY_WORKHORSE_API_KEY", "ASTRO_GATEWAY_WORKHORSE_BASE_URL"] (qualified)
        // or ["ASTRO_GATEWAY_API_KEY", "ASTRO_GATEWAY_BASE_URL"] when entry name == "astro-gateway".
        names := deployment.CredentialEnvVars(ds, modelName)
        for _, name := range names {
            switch {
            case strings.HasSuffix(name, "_API_KEY"):
                resolved.SecretData[name] = a.astroGatewayAPIKey
            case strings.HasSuffix(name, "_BASE_URL"):
                resolved.SecretData[name] = a.astroGatewayBaseURL
            }
        }
    }
}
```

`deployment.CredentialEnvVars(ds, modelName)` is a new thin helper that wraps the existing resolver logic — it's already exercised by `envresolver_test.go`, this just exposes it as a public function callable from the applier.

**The env-var names are the resolver's responsibility, not the applier's.** §1 declares the credential suffixes (`API_KEY`, `BASE_URL`); the resolver (after the small fix in §1.3 to emit names for managed providers) derives `<PROVIDER>_[<MODEL_NAME>_]<SUFFIX>` per the rule already in `packages/astro-spec/envresolver*.go`; this block just looks up which names the resolver picked for each `astro-gateway`-provider entry and fills the gateway values in. Adding a new credential suffix to `astro-gateway` in §1 (e.g. `MODEL_ID` later) means the new env var appears automatically — no edit here.

That's the entire surface in the apply path. The agent's existing build env, ConfigMap, Secret, Deployment all flow through unchanged — the `ASTRO_*` env vars are picked up by the agent container via the standard env-from-Secret pattern; the spec_applier resolution step puts them into the right Secret data map under the names the resolver computed.

### 4. Rotation — `internal/riverqueue/ai_gateway_rotate.go`

Daily periodic worker (added to `periodic.go` schedule). Two-stage state machine driven by columns we already added in the store:

- **Stage A (rotate-mint):** for accounts where `rotates_at < now AND prev_expires_at IS NULL`:
  1. `GenerateKey` → new `keyID2`, new plaintext.
  2. Move current → prev: `key_id_prev = key_id`, `encrypted_api_key_prev = encrypted_api_key`, `prev_expires_at = now + 7d`.
  3. Install new: `key_id = keyID2`, `encrypted_api_key = enc(plaintext2)`, `issued_at = now`, `rotates_at = now + 30d`.
  4. **Re-deploy not required.** The agent pod won't pick up the new value until the next deploy. The previous key remains valid for 7 days, so any agent revision deployed during that window still authenticates. Next deploy on the account naturally picks the new key.

- **Stage B (rotate-revoke):** for accounts where `prev_expires_at < now AND key_id_prev IS NOT NULL`:
  1. `DeleteKey(key_id_prev)` against LiteLLM.
  2. Null out `key_id_prev`, `encrypted_api_key_prev`, `prev_expires_at`.

Both stages are idempotent. Worker logs each transition. Failure to delete the prev key is logged but doesn't block; we re-attempt next run.

**Push-down rotation:** explicitly not in v1. Cutting agent pods on a rotation tick to pick up a new key is unnecessary because the previous key is still valid for 7 days — by the next normal deploy the agent has the new value. Saves us a fleet-wide restart story for v1.

### 5. Account teardown — `internal/riverqueue/purge_accounts.go`

Already deletes Langfuse projects. Add one call:

```go
if w.aigwProvisioner != nil {
    if err := w.aigwProvisioner.RevokeTenantKey(ctx, accountID); err != nil {
        w.log.Warn("ai-gateway key revoke failed", "account", accountID, "err", err)
    }
}
```

### 6. Remove `anthropic-managed`

`anthropic-managed` was the precedent for the Managed-provider mechanism, but it's strictly inferior to `astro-gateway` along every axis: same upstream (Claude on Anthropic-direct), no per-tenant attribution (everything bills to the platform's one Anthropic key), no rotation story, no quota story, no gateway-level guardrails. The `astro-gateway` provider supersedes it functionally. Keeping both around invites confusion ("which managed should I use?") and means we carry two code paths that do almost the same thing.

The removal is contained — a grep over the repo finds **only Go code** (provider registry, validator, applier, deployer, config, tests). Zero references in Helm values, Terraform, or shipped spec files. The blast radius is whatever user astropods.yml files in stored deployments use `provider: anthropic-managed`.

**Rollout — three PRs, gated on §4 (preview gateway enabled) landing first:**

**(a) Migrate stored specs.** One-shot maintenance job (river worker, runs once on deploy, then unregisters) that walks `deployment_specs` and rewrites `provider: anthropic-managed` → `provider: astro-gateway` with a default `model: claude-sonnet-4-6` when the spec didn't pin one. The rewrite is purely an in-place spec update — no redeploy is triggered; the new value takes effect on the account's next normal deploy, which then mints the gateway key and gets routed through it. Job logs every rewrite for audit; idempotent (no-op if no `anthropic-managed` rows remain). Runs in preview first, soaks one week, then prod.

**(b) Validator rejects new uses.** Once the migration job has drained preview (zero `anthropic-managed` references in `deployment_specs`), the validator gains a hard rule: `provider: anthropic-managed` is rejected at admission with `anthropic-managed is removed; use 'astro-gateway' or supply your own Anthropic key via the 'anthropic' provider`. Catches anyone trying to push a new spec from an old CLI / saved template. Spec parser keeps recognizing the name during the deprecation window so the validator's error message fires (vs. a less helpful "unknown provider").

**(c) Delete the code.** Once (a) has run successfully in prod and (b) has been live for two weeks with no rejections logged:

- Drop the `anthropic-managed` entry from `builtinProviders` in `packages/astro-spec/provider.go`.
- Drop `ManagedAnthropicAPIKey` from `internal/config/config.go` (and the `MANAGED_ANTHROPIC_API_KEY` env var binding).
- Drop the field from `ApplierConfig` in `internal/k8s/applier.go` and the wire-through in `internal/deployer/deployer.go:160`.
- Delete the validator rejection rule from (b) — the spec parser no longer recognizes the name, so the validator's "unknown provider" path handles it.
- Update tests in `internal/deployment/validator_test.go`, `template_test.go`, `internal/k8s/spec_applier_test.go`, `handlers/deploy_test.go`, `packages/astro-spec/envresolver_test.go` to use `astro-gateway` (or just delete the test case if it was specifically testing the legacy path).
- Update `handlers/deploy.go:721` and `handlers/deploy.go:4125` — the comments referencing `anthropic-managed` as an example of a managed provider — to reference `astro-gateway` instead.
- Delete the `MANAGED_ANTHROPIC_API_KEY` Secrets Manager entry from preview and prod env states (separate astro-infra PR).

**Why this ordering matters.** If we delete the spec entry before (a) runs, every existing `anthropic-managed` spec in the DB starts failing validation on the next load (e.g. when a deployment is restarted, or when the deploy detail page renders the spec). Migration first means at the point of code deletion, no row in `deployment_specs` references the dead name and no live agent is depending on it.

## Config wiring

Follow the existing pattern in `internal/config/config.go`: **flat fields on `DeploymentConfig`**, loaded via `getEnv`, with **emptiness as the disable signal** (no separate `Enabled` bool). Sensitive values arrive as env vars from External Secrets Operator — no Secrets Manager API call at boot. KMS reuses the existing `Cfg.Deployment.KMSKeyARN`. Same shape as the Langfuse fields right next to them at lines 206–211.

Env vars are already wired in the astro-server helm chart:

```yaml
- name: AI_GATEWAY_URL
  value: "${ai_gateway_url}"
- name: AI_GATEWAY_MASTER_KEY
  valueFrom:
    secretKeyRef:
      name: astro-server-ai-gateway
      key: AI_GATEWAY_MASTER_KEY
```

Append to `DeploymentConfig` in `internal/config/config.go` (matching the flat Langfuse pattern at lines 206–211):

```go
// AI Gateway — per-tenant virtual key issuance against LiteLLM.
AIGatewayURL       string // AI_GATEWAY_URL — LiteLLM endpoint astro-server calls /key/generate against
AIGatewayMasterKey string // AI_GATEWAY_MASTER_KEY — LiteLLM master key (from ESO-delivered Secret)
```

And the matching `getEnv` lines in `Load()`:

```go
AIGatewayURL:       getEnv("AI_GATEWAY_URL", ""),
AIGatewayMasterKey: getEnv("AI_GATEWAY_MASTER_KEY", ""),
```

Notes on what's *not* in this config block:

- **No env / region stamp.** Each environment runs its own gateway ([ai-gateway.md §"Where it runs"](../../modules/astro-infra/docs/plans/ai-gateway.md) — preview's `ai_gateway.tf` and prod's are entirely separate states, separate ALBs, separate Postgres). The gateway astro-server talks to *is* the env, by construction. No need to thread a `preview`/`prod` string through anywhere, and no risk of a preview key accidentally validating against the prod gateway (different master keys, different RDS instances).
- **KMS key for envelope-encrypting stored virtual keys** = the existing `Cfg.Deployment.KMSKeyARN` from `KMS_KEY_ARN`. The infra side allocates a dedicated CMK for the gateway, but astro-server doesn't need a separate handle for it — the deployment-level key the deployer already uses is the same one that gates access to gateway-side secrets. If we want isolation later, that's a follow-up.
- **No "public base URL" field.** `AI_GATEWAY_URL` is the *one* URL astro-server knows about, both for its own admin calls *and* as the value we write into the tenant Secret's `base_url` slot. In preview's single-cluster topology, in-cluster tenants resolve the same Service the astro-server admin calls hit — `http://ai-gateway.ai-gateway.svc.cluster.local:4000` (set in `terraform/environments/preview/helm.tf:295`). The public ALB at `aig.astropod.ai` is for out-of-cluster callers (dev laptops, CI, and eventually BYOC). When prod adds BYOC tenants in a different cluster, we'll add a second env var carrying the public URL and have the provisioner return that as `baseURL` — until then the single URL covers both code paths and the master key never traverses the public path.

**Construction site is `internal/riverqueue/workers.go`**, not `main.go` — the same place Langfuse is wired today (line 61). Add the parallel block right after the Langfuse one:

```go
// Initialize AI Gateway per-account provisioning if configured
if cfg.ServerConfig.Deployment.AIGatewayURL != "" {
    dep.AIGatewayStore = aigateway.NewStore(cfg.DB)
    prov, err := aigateway.NewProvisioner(
        cfg.ServerConfig.Deployment.AIGatewayURL,
        cfg.ServerConfig.Deployment.AIGatewayMasterKey,
        cfg.ServerConfig.Deployment.KMSKeyARN,
        cfg.KMSClient,
    )
    if err != nil {
        cfg.Logger.Warn("Failed to initialize AI Gateway provisioner", "error", err)
    } else {
        dep.AIGatewayProvisioner = prov
    }
}
```

Same fire-and-forget shape as Langfuse: any startup error gets logged and the provisioner stays nil. Deploys with `provider: astro-gateway` then fail at the `EnsureTenantKey` call (§3) — surfacing a clear "gateway not configured" error rather than a corrupted attribution path.

**When disabled** (`AI_GATEWAY_URL` unset — e.g. in dev, or in an environment where the gateway hasn't been rolled out): `dep.AIGatewayProvisioner` is nil. The deployer-side check `if d.AIGatewayProvisioner != nil && deploymentReferencesAstroGateway(ds)` (§3) skips the ensure call, and the validator gate below rejects the spec at admission so we don't ship a pod with empty `ASTRO_GATEWAY_*` env vars.

**Validator gate.** When `provider: astro-gateway` is used but `AIGatewayURL` is empty, reject at admission with `astro-gateway provider not enabled in this environment — supply your own provider credentials via the 'anthropic' or 'openai' providers`. The validator reads the config directly (same way it consults `ManagedAnthropicAPIKey` today via `DeploymentConfig`); no new plumbing needed.

## Rollout order (preview)

Each step is independently revertible; the feature is gated behind the absence of `AI_GATEWAY_URL` (empty = provisioner not constructed, validator rejects `provider: astro-gateway` at admission) so steps 1–4 can ship dark.

1. **Migration + store + client.** PR adds the `account_ai_gateway` table and the `internal/aigateway/` package with unit tests against a faked LiteLLM admin. No call sites yet.
2. **Provider declaration + resolver fix + validator.** PR adds the `astro-gateway` builtin in `packages/astro-spec/provider.go`, removes the `Managed` skip at `envresolver.go:213` so credential env-var names *are* emitted for managed providers (`anthropic-managed`'s hardcoded `ANTHROPIC_API_KEY` in spec_applier stops being load-bearing once §6 lands), and adds the validator gate (model whitelist + enabled-flag check). Spec parses, resolver emits qualified credential names, validator rejects when flag is off. The pre-existing `anthropic-managed` hardcoded injection keeps working in parallel — it'll be deleted by §6.
3. **Deployer + applier wiring.** PR threads `AIGatewayProvisioner` into `deployer.Deployer`, adds the ensure call + applier-config fields + the spec_applier injection. Still dark in prod; behind the flag.
4. **Enable in preview.** The astro-server helm chart already wires `AI_GATEWAY_URL` (from `${ai_gateway_url}`) and `AI_GATEWAY_MASTER_KEY` (from the `astro-server-ai-gateway` Secret via ESO). Once both are non-empty on the preview deployment, astro-server constructs the provisioner on next restart. Smoke test: pick a preview agent, switch one model to `provider: astro-gateway`, redeploy, watch the agent's pod env, watch OpenMeter `/api/v1/meters/ai_requests/query?subject=<keyID hash>` light up. (Same end-to-end check called out in [ai-gateway.md Phase 4](../../modules/astro-infra/docs/plans/ai-gateway.md), now from the astro-server side.)
5. **Rotation worker.** PR adds the river worker, registers it on the periodic schedule. First rotation tick is 30 days out, so we have plenty of preview soak before any rotation actually fires.
6. **Purge integration.** Adds the `RevokeTenantKey` call into `purge_accounts.go`.
7. **`anthropic-managed` removal (a) — migration job.** Runs the spec rewrite. Soak one week in preview.
8. **`anthropic-managed` removal (b) — validator rejection.** Hard error on new uses. Soak two weeks.
9. **`anthropic-managed` removal (c) — code deletion.** Drops the builtin, config, applier field, and tests.

Prod rollout = repeat 1–9 against prod env state once preview has 2+ weeks clean, same gating model. Steps 7–9 specifically need preview's `anthropic-managed` row count to hit zero before they can fire in prod.

## Out of scope (v1)

- **Per-agent (not per-account) keys.** Today one key per tenant covers every agent the tenant deploys. If we ever want per-agent attribution inside a tenant, that's a `team_id` change on the gateway side + a schema change here; not blocking v1.
- **Push-down rotation that restarts agent pods on a rotation tick.** See §4 above — 7-day overlap window makes restarts unnecessary, and we avoid having to invalidate every revision the moment a rotation runs.
- **BYOK (tenant supplies their own Anthropic / OpenAI key, routed through the gateway).** Plan-level non-goal in ai-gateway.md; just noting it here so spec authors who ask "can I bring my key" get pointed at the existing `anthropic` / `openai` (non-managed) builtins.
- **Per-deploy cost preview in the UI.** OpenMeter has the data; surfacing it on the deploy detail page is a follow-up in `astro-client`, not this plan.
- **Cross-region key migration.** When prod gets multi-region, a tenant's key needs to be valid in every region's gateway. Resolved by the gateway side (single key, multi-region replication of the LiteLLM Postgres) rather than minting per-region keys here. Capture as a follow-up the day we have a second region.

## Risks

- **Deploy fails when gateway is down.** Fail-hard semantics mean a gateway outage = no new deploys for agents using `provider: astro-gateway`. Mitigations: the existing agent revision keeps running with its existing key (Kubernetes doesn't restart pods on a deploy failure), so live traffic is unaffected; only the *next* push is blocked. Operators can fall back to a BYOK provider (`anthropic`, `openai`) per-agent if the outage drags on.
- **Stored key gets out of sync with LiteLLM's Postgres** (e.g. someone runs `/key/delete` manually). Mitigation: a thin reconciler — post-v1 — walks `account_ai_gateway` rows and verifies each `key_id` is alive via `/key/info`, alerts on drift. v1 lives with manual-cleanup risk; the surface area is small.
- **Validator model whitelist drifts from the gateway's actual `model_list`.** Two sources of truth — `internal/aigateway/models.go` and `helm/values/common/ai-gateway.yaml.tpl`. Mitigation in v1: a unit test that loads the chart's values template and diffs the model lists; CI fails on drift. Long-term, expose the gateway's `/v1/models` and resolve once at server startup so the source of truth is the gateway itself.
- **Per-account key leakage in logs.** The deployer + applier paths pass plaintext keys around briefly. Mitigation: standard `slog` handler scrubs values whose keys match `*_API_KEY` (already covers the `ASTRO_GATEWAY_*_API_KEY` pattern). Verify with a grep test in CI that no plaintext key matching the LiteLLM prefix (`sk-astro-`) appears in any captured log fixture.
