# Unified Deployment Environment Specification

**Version**: 0.2
**Status**: Draft — for review
**Date**: 2026-04-28

## Overview

Every environment variable that any container in a deployed agent receives — user variables, provider-managed credentials, service URLs, platform metadata, signed tokens — is **resolved during template processing** and stored in a single table keyed by `(deployment, role, env_name)`. Kubernetes ConfigMaps and Secrets are a pure projection of that table at apply time. No re-resolution at apply time. No value lives in two places. No precedence rules at the Kubernetes layer.

This replaces the current pipeline, where:
- `template.go` auto-injects `${knowledge.x.credentials.y}` references into `agent.Environment`.
- `spec_resolver` materialises those into the agent's full credentials Secret as resolved values.
- `spec_applier`'s `knowledgeCredEnvVars` *also* wires the same names via per-store `secretKeyRef`.
- The same credential ends up on the agent container twice — once via `envFrom` of the agent Secret, once via direct `env`. K8s precedence (direct wins) hides this at runtime, but the duplicate surfaces in the API and trips the React-key bug fixed under `fix/dedupe-container-env`.

## Terminology

- **Role** — a logical container slot in a deployed agent. One row per role per env name:
  - `agent` (the user's app), exactly 1
  - `messaging` (sidecar in the agent pod), 0..1
  - `collector` (telemetry sidecar), 0..1
  - `knowledge:<name>` — one per declared knowledge store, 0..N
  - `ingestion:<name>` — one per declared ingestion (job/cronjob/webhook deployment), 0..N
- **Resolution** — converting a spec-level value (literal or `${…}` reference) into a concrete string the container will see at runtime.
- **Resolved row** — a `(deployment_id, role, env_name, value, is_secret, source)` tuple stored in `deployment_build_env`. The atomic unit of "the agent will see X=Y."
- **Projection** — the apply-time step that reads all resolved rows for a `(deployment, role)` and writes a ConfigMap (non-secret rows) and Secret (secret rows). The container is wired with `envFrom` of those two and *nothing else*.
- **Provider-managed credential** — auto-generated value for a self-hosted store (postgres user/password/db; redis password). Live only in the per-store K8s Secret today; under this spec, materialised into resolved rows for any role that needs them.
- **User-declared variable** — a value the user enters in the deploy form. Encrypted at rest in the deployment record today; under this spec, materialised into resolved rows the same way provider creds are.
- **`build_id`** — content hash of the agent image. Used by the applier for K8s resource naming so rolling updates between code versions can isolate old and new pods. **Not stored on env rows.** See "Why build_id is not in the key" below.

## Goals

1. **Single source of truth per env value**: `(deployment, role, env_name)` is unique. The schema itself rules out the duplicate-row class of bug.
2. **Apply is mechanical**: `spec_applier`'s env wiring becomes "read rows, write ConfigMap+Secret, mount via envFrom." No knowledge of variables, knowledge stores, providers, adapters, or `${…}` syntax.
3. **All resolution in one place**: one `Resolve` function understands the spec language. Pure function; no K8s, no DB. Exhaustively unit-testable.
4. **Current-intent state**: the table holds the deployment's current intended environment. Mutations (variable rotation, knowledge cred re-resolution) are in-place updates. Build-version transitions are handled at the K8s naming layer, not in the DB.
5. **Honest API**: the deployment-detail API reports exactly what the container sees, one row per env name, with a categorical `source` rather than name-based heuristics.
6. **Smaller blast radius**: a leak of the agent's K8s Secret no longer exposes provider credentials — those are no longer in it.

## Non-Goals

1. **Pod-runtime-only env** like `valueFrom: fieldRef: status.podIP`. There's no value to resolve at template time. Keep these as a small fixed list in the applier (3–4 across the platform). Not modelled in the table.
2. **Modelling every K8s feature** (`resourceFieldRef`, `serviceAccountToken` projections, etc.). The table is for resolved string values; niche K8s primitives stay in code.
3. **Per-build env history on the hot path**. The hot table holds current intent only. If cross-build env history is ever needed (debugging, audit, rollback), it lives in audit-log replay or a separate snapshot table — see "Rollback and history."
4. **Replacing the spec language**. `${variables.X}`, `${knowledge.X.host}`, `${knowledge.X.credentials.Y}` keep working in user-written `agent.environment` etc. They get resolved earlier, not differently.

## Schema

This spec adds one new table and subsumes two existing ones (`deployment_variables` and `deployment_resolved_keys`).

### Layering

```
┌─────────────────────────────────────────────────────────────────┐
│ User input (deploy form)                                        │
│   variable name, value, secret?, optional?, account-var ref     │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼   Resolve(spec, ctx) — pure function
                              merges user variables with platform-emitted
                              rows (knowledge creds, service URLs, platform
                              meta, auth tokens, adapter config)
                          │
┌─────────────────────────────────────────────────────────────────┐
│ deployment_build_env  (NEW — single source of truth)            │
│   "what each container will actually see, right now"            │
│                                                                 │
│   key:   (deployment_id, role, env_name)                        │
│   cols:  value_encrypted, nonce, is_secret, source,             │
│          user_var_name?, account_var_ref?, optional?            │
│                                                                 │
│   Current intent. K8s ConfigMap and Secret are a pure           │
│   projection of these rows.                                     │
└─────────────────────────────────────────────────────────────────┘
                          │
                          ▼   Apply(deployment, build_id)
                              for each role:
                                project rows → ConfigMap + Secret
                                K8s resource name uses build_id
                                container.envFrom = [cm, sec]
                          │
                          ▼
                       running pods
```

### deployment_build_env (new)

```sql
CREATE TABLE deployment_build_env (
    deployment_id    TEXT         NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    role             TEXT         NOT NULL,        -- 'agent' | 'messaging' | 'collector' | 'knowledge:<name>' | 'ingestion:<name>'
    env_name         TEXT         NOT NULL,        -- env var name as the container sees it

    value_encrypted  BYTEA        NOT NULL,        -- AES-GCM ciphertext, deployment data key
    nonce            BYTEA        NOT NULL,
    is_secret        BOOLEAN      NOT NULL,        -- true → projects to Secret; false → ConfigMap
    source           TEXT         NOT NULL,        -- 'user_var' | 'platform_meta' | 'service_url' | 'knowledge_cred' | 'auth_token' | 'adapter_config' | 'derived'

    -- Provenance for user_var rows. NULL on platform-emitted rows.
    user_var_name    TEXT,                         -- canonical user variable this row came from
    account_var_ref  TEXT,                         -- original ${account.var.X} ref, for prefill
    optional         BOOLEAN,                      -- copied from user variable declaration

    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    PRIMARY KEY (deployment_id, role, env_name)
);

-- Reconstruct the deploy form's "what variables exist" view
CREATE INDEX deployment_build_env_user_vars
  ON deployment_build_env (deployment_id, user_var_name)
  WHERE source = 'user_var';
```

### Why `build_id` is not in the key

`build_id` is the content hash of the agent image. It changes only when the user pushes new code. Env state is independent of `build_id`:

- A user rotating a token without rebuilding the agent image keeps the same `build_id`, but the env value changes. Putting `build_id` in the key would force us to overload its meaning or invent a "revision" sub-axis.
- Rolling-update isolation between two `build_id`s is already handled at the K8s layer: the ConfigMap and Secret resource *names* embed `build_id` (`sasbot-<build>-config`), so old pods literally point at a different K8s object than new pods. The applier owns that naming. The DB doesn't need to mirror it.
- Cross-build env history (rollback, debugging) is rare. We don't pay for it on every write to the hot path. See "Rollback and history."

The DB stores **what the deployment's env should be right now**. The applier maps that, plus the current `build_id`, to K8s resources.

### Field rationale

- **`role`** is a string with a small grammar (`<kind>` or `<kind>:<name>`) rather than two columns. Keeps the primary key a single tuple and avoids null-handling for the optional name component.
- **All values encrypted**, including non-secrets. The deployment data key is already per-deployment; encrypting universally keeps the storage layer uniform and avoids the "did I forget to encrypt this one" class of bug. Cost is negligible — projection happens at apply time, not on every read.
- **`is_secret`** is the only signal the projector and the API need. Replaces the spec_resolver's `ConfigMapData` / `SecretData` bifurcation and the UI's name-based `isSensitiveEnvVar` heuristic.
- **`source`** is metadata for humans (audit, deploy UI). The applier ignores it. New source values can be added without schema change.
- **`user_var_name` / `account_var_ref` / `optional`** carry the deploy-form metadata for `source='user_var'` rows so the form can reconstruct itself without a second table. NULL on platform-emitted rows; saves nothing in storage cost (TOAST handles short nullable text fine) and makes the schema honest about which rows have which provenance.
- **No `value_kind` / `ref_target` columns**. We considered storing references (e.g. "this row points at knowledge-postgres-creds.POSTGRES_USER") rather than literals so credential rotation could update one row. Rejected: re-introduces the apply-time resolution we're trying to remove. Rotation just updates the row.

### Subsumed: `deployment_variables`

Today's `deployment_variables` table holds user-declared variables: `(deployment_id, name, value, ref, secret, optional, targets, nonce)`. Under this spec it folds entirely into `deployment_build_env`:

| `deployment_variables` column | Where it goes |
|---|---|
| `name` | `user_var_name` on the resulting rows |
| `value` / `nonce` | `value_encrypted` / `nonce` |
| `secret` | `is_secret` |
| `optional` | `optional` |
| `ref` | `account_var_ref` |
| `targets` | *implicit* — encoded by which `(role, env_name)` rows exist with the same `user_var_name` |

A single user variable that targets `agent` + `interface.slack` produces two rows (`role='agent'`, `role='messaging'`) with the same `user_var_name` and identical value. The deploy form's "what variables does this deployment have" query becomes:

```sql
SELECT DISTINCT user_var_name, account_var_ref, is_secret, optional
FROM deployment_build_env
WHERE deployment_id=$d AND source='user_var';
```

Variable rotation:

```sql
UPDATE deployment_build_env
SET value_encrypted=$v, nonce=$n, updated_at=NOW()
WHERE deployment_id=$d AND user_var_name='ANTHROPIC_API_KEY';
-- Hits all rows where this var fans out (e.g. agent + ingestion). One transaction.
```

There is no second table to keep in sync because there is no second table.

### Subsumed: `deployment_resolved_keys`

The existing `deployment_resolved_keys` table tracks the expected ConfigMap/Secret key sets per deployment, with SHA-256 hashes per value, for drift detection against live K8s. Under this spec it becomes redundant: `deployment_build_env` rows are the authoritative key set per role, and value hashes can be computed on demand from the encrypted column. Drift detection is "project rows for `(deployment, role)` and diff against the live ConfigMap+Secret pair." Phase 2 of the migration removes `deployment_resolved_keys`.

## Lifecycle

```
1. Template
   astropods.yml + user inputs + selected adapters
        │
        ▼
   buildDeploymentSpec(template.go)  ← unchanged: produces ds.AstroDeploymentSpec.
                                        Spec still carries declarative ${} refs;
                                        they are now metadata, not load-bearing.

2. Resolve  (NEW)
        │
        ▼
   resolutions := Resolve(ds, ResolveContext{
       UserInputs, BoundCreds, ServiceCoords, PlatformMeta, AuthTokens,
   })
   // Pure function. Returns []Resolution. No I/O.

3. Persist  (NEW — same transaction as the deployment record)
        │
        ▼
   for r in resolutions:
       INSERT INTO deployment_build_env (…, encrypt(r.Value), …)
       ON CONFLICT (deployment_id, role, env_name)
       DO UPDATE SET value_encrypted=…, nonce=…, is_secret=…, source=…,
                     user_var_name=…, account_var_ref=…, optional=…,
                     updated_at=NOW()
   -- Plus DELETE rows that no longer exist in the new resolution.
   -- All in one transaction. Idempotent.

4. Apply  (spec_applier becomes mechanical)
        │
        ▼
   for role in roles_for(deployment):
       rows := SELECT … WHERE deployment_id=$d AND role=$r
       cm   := ConfigMap{
                 name: "<agent>-<build_id>-<role>-config",
                 data: { r.env_name: decrypt(r) | r ∈ rows, ¬r.is_secret }
               }
       sec  := Secret{
                 name: "<agent>-<build_id>-<role>-credentials",
                 data: { r.env_name: decrypt(r) | r ∈ rows,  r.is_secret }
               }
       container := template_for(role)
       container.envFrom = [cm, sec]    // the ONLY env wiring
       container.env     = []           // empty; nothing direct
       pod_template.annotations["astro.dev/env-hash"] = hash(rows)
```

The applier never sees `${…}` syntax, never resolves credentials, never makes a precedence decision. If a value isn't in `deployment_build_env`, it's not in the container. `build_id` enters the picture only as a string in K8s resource names so two coexisting builds during a rolling update don't collide.

## Resolution rules

`Resolve(ds, ctx)` walks the deployment spec and emits one `Resolution` per `(role, env_name)`. A non-exhaustive map of where rows come from:

| Source | Driven by | `is_secret` | Notes |
|---|---|---|---|
| `user_var` | `ds.Variables[X]` where `Targets` includes role | per `Variable.Secret` | The user's deploy-form input. Carries `user_var_name`, `account_var_ref`, `optional`. |
| `platform_meta` | runtime: `ASTRO_AGENT_NAME`, `_BUILD`, `_HOST`, `_URL` | false | Derived from deployment metadata. |
| `service_url` | knowledge/integration DNS coords | false | `${knowledge.X.host}`/`.port`/`.url` references. |
| `knowledge_cred` | per-store auto-generated creds | true | `${knowledge.X.credentials.Y}` references; per-store renaming (`POSTGRES_USERS_USER`) is a row-naming concern handled here, not in the applier. |
| `auth_token` | platform-signed JWT (`ASTRO_AUTHZ_TOKEN`) | true | Issued at template time. |
| `adapter_config` | inlined adapter config (`SLACK_CONFIG`) | false (today) | Driven by `interfaces.environment`. |
| `derived` | escape hatch for synthesized values that don't fit above | per-row | E.g. `OTEL_EXPORTER_OTLP_ENDPOINT` from collector coords. |

Resolution is **per role**, not per spec. The agent's `POSTGRES_USERS_USER` and the users-store knowledge container's `POSTGRES_USER` are independent rows that happen to carry the same value. They have different `env_name`s because they live in different roles.

### Targets are load-bearing

A variable's `Targets` directly determines which roles get rows for it. `Resolve` MUST consult `Targets` (or the equivalent for platform-emitted rows) before emitting a row. This is what makes cross-role leaks structurally impossible:

```
ds.Variables["SLACK_BOT_TOKEN"]  Targets=["interface.slack"]
   → row for role='messaging'
   → NO row for role='agent'

ds.Variables["ANTHROPIC_API_KEY"] Targets=["agent"]
   → row for role='agent'
   → NO row for role='messaging'

ds.Variables["DB_URL"]            Targets=["agent","ingestion"]
   → rows for role='agent' and every role='ingestion:<name>'
   → NO row for role='messaging'
```

The agent's projected Secret only carries rows where `role='agent'`. Slack values can't land in it because their `Targets` doesn't include `agent`. Today's behavior — `SLACK_BOT_TOKEN`, `SLACK_CONFIG`, and every other secret variable lumped into one shared bundle the agent envFroms — is a direct consequence of the resolver ignoring `Targets`. Under this spec, the resolver respects `Targets` and the schema makes the scoping authoritative.

This is also what subsumes the `12a294ac` carve-out (scoping the messaging container to its own narrow Secret). That fix was one-directional — it tightened messaging's view but left the agent with everything. Per-role rows tighten both sides by the same mechanism.

## Cardinality example

A realistic deploy (your `sasbot` from this conversation: agent + messaging + collector + 3 knowledge stores) produces, conservatively:

| Role | Rows | Detail |
|---|---|---|
| `agent` | ~28 | 14 connection coords (HOST/PORT/URL × 3 stores + ASTRO_AGENT_* + GRPC_SERVER_ADDR + OTEL endpoint + SLACK_CONFIG); 9 user-declared secrets (ANTHROPIC, GITHUB, CLOUDFLARE_*, SLACK_*, POSTGRES/REDIS user-cred env names per store); 1 auth token; 4 platform-meta. **No duplicates.** |
| `messaging` | 12 | adapter knobs + auth token + adapter-config |
| `collector` | 5 | langfuse coords + agent identifiers |
| `knowledge:postgres` | 3 | POSTGRES_USER/PASSWORD/DB |
| `knowledge:users` | 3 | same |
| `knowledge:cache` | 1 | REDIS_PASSWORD |

Total: ~52 rows for the whole deployment. Each row deterministically derived from the spec + context.

## Rollback and history

Rollback is the only argument for keeping `build_id` partitions on the hot path, and it's a weak one — rollback is rare, the hot path runs constantly. Two options, both cheaper than partitioning every row:

1. **Audit-log replay.** Writes to `deployment_build_env` are logged to `auditlog` (with `resource='deployment_env'`, the row PK, and the prior value). Rollback to time T = "find writes since T and reverse them." Concrete, no schema growth, and free for the 99% of deployments that never rolls back.
2. **Snapshot table** (added later if needed):

   ```sql
   CREATE TABLE deployment_build_env_snapshot (
       snapshot_id     UUID PRIMARY KEY,
       deployment_id   TEXT NOT NULL,
       build_id        TEXT NOT NULL,
       captured_at     TIMESTAMPTZ NOT NULL,
       payload         JSONB NOT NULL  -- whole projection at this point in time
   );
   ```

   Written only at "deploy" events (not every mutation). Cardinality stays small.

Either way, the hot table stays current-only.

## Migration

Single PR. No feature flag, no parallel code paths, no boot-time backfill worker, no operator orchestration tooling. The migration happens **lazily inside the applier**: the first time a deployment is applied after the cutover, the applier resolves and persists its rows. Reconcile already runs every ~10 minutes, so within one reconcile cycle every active deployment has migrated itself.

### The applier change

```
applyEnv(deploymentID, buildID):
    rows := SELECT … FROM deployment_build_env WHERE deployment_id=$d
    if len(rows) == 0:
        // First apply for this deployment under the new code. Read inputs
        // we already have, run Resolve, write rows. Same transaction.
        ds   := parse(deployment.deployment_spec_json)
        vars := SELECT * FROM deployment_variables WHERE deployment_id = $d
        ctx  := build_resolve_context(deployment, vars, bound_knowledge_state)
        rows  = Resolve(ds, ctx)
        INSERT INTO deployment_build_env (…) VALUES (…)

    for role in roles_for(deployment):
        rs := rows where role=$r
        cm := ConfigMap{ name: "<agent>-<build>-<role>-config",
                         data: { r.env_name → decrypt(r.value) | r ∈ rs, ¬r.is_secret } }
        sec := Secret{   name: "<agent>-<build>-<role>-credentials",
                         data: { r.env_name → decrypt(r.value) | r ∈ rs,  r.is_secret } }
        container.envFrom = [cm, sec]
        container.env     = nil
```

Single code path. The "is this deployment migrated yet" question doesn't exist as a separate concept — if rows are missing, the applier creates them on the spot. Once they exist, every subsequent apply just reads.

### The PR

1. **DDL migration**: create `deployment_build_env`. Do *not* drop `deployment_variables` or `deployment_resolved_keys` yet — they're inputs to the lazy resolve, and serve as a rollback artifact.
2. **`Resolve` function** + projector implemented.
3. **`spec_applier` env wiring** rewritten to the snippet above. `knowledgeCredEnvVars`, the per-store cred-Secret-on-agent hookup, the messaging-only Secret carve-out — all deleted. There is no second code path.
4. **`spec_resolver`'s `ConfigMapData` / `SecretData`** split removed; `Resolve` is now the only resolver.
5. **`template.go`** stops auto-injecting `${knowledge.x.credentials.y}` into `agent.Environment`.
6. **`buildContainerStatuses`** keeps the dedupe defence from `fix/dedupe-container-env` as belt-and-suspenders.
7. **UI Variables tab** uses `is_secret` + `source` from the resolved rows, dropping the `isSensitiveEnvVar` heuristic.

### Pre-merge confidence

`Resolve` is a pure function. Exhaustive unit tests against fixtures derived from real production deploy specs are the primary safety net. A small CLI harness (`go run ./cmd/env-dryrun --deployment-spec=… --vars=…`) makes it easy to run those resolutions and diff against live K8s during development. No queen, no admin RPC — it's a developer tool, not infrastructure.

### Side effects when this PR lands

The K8s ConfigMap and Secret shape per container changes:

- The agent's projected Secret no longer carries SLACK / messaging-only / ingestion-only secrets.
- Per-store cred values move from `secretKeyRef` to `envFrom`-resolved (or vice versa, depending on the entry). Same values, different wiring.

Per-role ConfigMap and Secret objects appear under new names (`<agent>-<build>-<role>-…`); the old whole-deployment objects (`<agent>-<build>-config`, `<agent>-<build>-credentials`) become orphaned and are cleaned up by `orphan_cleanup`.

The `astro.dev/env-hash` annotation changes for every running pod → **rolling restart of every running deployment's pods.** Same env values reach the application code, but the pods cycle. Coordinate with a deploy window.

### If the cutover misbehaves

The same channels we use for any other regression:

- Find the bug in `Resolve` (or the projector, or the applier path).
- Fix it; deploy a new astro-server build.
- Next reconcile picks up the fix; affected deployments re-resolve on their next apply.

If the bug is bad enough to need a hard revert: revert the PR. `deployment_variables` and `deployment_resolved_keys` are still present in the schema for the safety-net window, so the old applier path keeps working when reverted.

### Cleanup (small follow-up PR, ~1 week later)

Once the new path has been running stably:

- `DROP TABLE deployment_variables;`
- `DROP TABLE deployment_resolved_keys;`
- Per-store cred K8s Secret as its own K8s object goes away — the knowledge container reads its rows like every other role.

## Edges and open questions

1. **Edit-without-rebuild** for variables that don't require new agent code (rotating `ANTHROPIC_API_KEY` via the deploy form): single `UPDATE` against `deployment_build_env`, re-project the affected role's ConfigMap/Secret, bump the pod's `astro.dev/env-hash` annotation. The hash is computed over current rows for `(deployment_id, role)`. No second table to update.

2. **Rolling code update** (new `build_id`): Resolve runs against the new spec; rows get `INSERT … ON CONFLICT DO UPDATE` (most rows unchanged, some updated, some inserted, deletes for removed roles). Apply names new ConfigMap and Secret with the new `build_id`. K8s rolls: new pods point at new resources, old pods at old. When the rollout finishes, old K8s resources are garbage-collected. The DB never had two versions of any row.

3. **User-written `secretKeyRef` in `agent.environment`**: the spec language doesn't support this today (only `${…}` refs to internal types). If we ever add it, it bypasses the resolved-rows model. Decision: don't add it; users write a `${…}` ref instead and the resolver materialises the value into a row.

4. **Agent's full Secret leaks**: today, an attacker reading `sasbot-cd092b41-credentials` gets ANTHROPIC_API_KEY *and* POSTGRES_PASSWORD *and* REDIS_PASSWORD. After this spec, the agent's projected Secret holds only the env names the agent role actually needs (with renaming applied — `POSTGRES_USERS_USER` etc.), not the per-store full bundle.

5. **Audit log**: writes to `deployment_build_env` should be logged. Current `auditlog` table can carry these as `resource='deployment_env'` events with the row PK. Resolution diff between updates becomes a useful per-deployment-changelog feature for free, and is the substrate for audit-log replay rollback.

6. **Performance**: ~50–100 rows per deployment; insert/upsert is one transaction at template time. Apply reads with one query per role (~6 queries per deployment). No hot path concern.

## What this kills

- `knowledgeCredEnvVars` and the per-store `secretKeyRef`-on-agent wiring → gone.
- The agent's full Secret carrying provider creds → gone (it carries only user-declared agent-targeted vars).
- `spec_resolver`'s `ConfigMapData` / `SecretData` two-bucket return → replaced by `[]Resolution`.
- `deployment_variables` table → folded into `deployment_build_env`.
- `deployment_resolved_keys` table → subsumed (rows carry both keys and value provenance; hashes computed on demand).
- Messaging-only Secret carve-out → gone (messaging projects from its own rows).
- `buildContainerStatuses` precedence / dedupe responsibility → structurally gone.
- `isSensitiveEnvVar` name-heuristic in the UI → replaced by authoritative `is_secret` + `source`.
- The duplicate-row class of bug → impossible at the schema level.
- The "two tables in lockstep" risk between user-input and resolved env → impossible (one table).
- Cross-role env leaks (e.g. `SLACK_BOT_TOKEN`, `SLACK_CONFIG` ending up on the agent because today's resolver ignores `Targets`) → impossible. Each row is keyed by role, written only where `Targets` says it belongs.

## What this preserves

- Rolling-update semantics (handled at K8s naming layer using `build_id`).
- The spec language (`${variables.X}`, `${knowledge.X.host}`, `${knowledge.X.credentials.Y}`).
- Encryption at rest with the deployment data key.
- Deploy-form prefill correctness — `account_var_ref` carries the original `${account.var.X}` reference per row.
- Existing audit-log and observability touchpoints.
- The `astropods.yml` user surface.

## Open work after agreement

**astro-server:**

- `Resolve` function: signature, pure-function shape, exhaustive unit tests against fixtures derived from real deploys.
- Applier rewrite: lazy-resolve on missing rows; projection is `envFrom = [cm, sec]; env = nil`.
- Template change: stop auto-injecting credential refs into `agent.Environment`.
- API: `buildContainerStatuses` reads the resolved table.
- Audit-log emission for `deployment_build_env` writes.
- `cmd/env-dryrun` CLI harness for development-time resolve+diff.

**astro-client:**

- Variables tab uses authoritative `is_secret` + `source`. Remove `isSensitiveEnvVar` heuristic.

**Cleanup PR (~1 week post-cutover):**

- Drop `deployment_variables`, `deployment_resolved_keys`.
- Drop the per-store cred K8s Secrets (knowledge containers read their projected Secret).
