# Backfill + dual-write `deployment_build_env` from `deployment_variables`

## Summary

Adds the `deployment_build_env` table from the unified-env spec **additively** — no behavior change, no consumers wired in. Two writers populate it:

1. A one-shot **backfill** that copies existing `deployment_variables` rows into the new shape (catches every deployment that exists at merge time).
2. A **dual-write** in `SaveNormalizedSpec` that mirrors every new variable insert into `deployment_build_env` (catches every deployment created or updated after merge).

Together they close the window where a deploy lands between the migration's `CREATE TABLE` and the next backfill run. By the time #848 (which drops `deployment_variables`) merges, every deployment has rows in `deployment_build_env`.

## Design

The new table:

```sql
CREATE TABLE public.deployment_build_env (
    deployment_id varchar(11) NOT NULL,
    role varchar(64) NOT NULL,
    env_name varchar(255) NOT NULL,
    value_encrypted bytea NOT NULL,
    nonce bytea,
    is_secret boolean NOT NULL,
    source varchar(32) NOT NULL,
    user_var_name varchar(255),
    account_var_ref text,
    optional boolean,
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    PRIMARY KEY (deployment_id, role, env_name),
    FOREIGN KEY (deployment_id) REFERENCES public.deployments(id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_build_env_user_vars
    ON public.deployment_build_env (deployment_id, user_var_name)
    WHERE source = 'user_var';
```

### Dual-write

`SaveNormalizedSpec` now writes user-variable rows to both tables in the same transaction:

- `deployment_variables` — unchanged shape; legacy consumers (RehydrateSecrets, admingrpc, prefill) keep working.
- `deployment_build_env` — fan-out to one row per (variable, target_role) pair via `rolesForBuildEnvTargets`. Same target → role mapping the backfill worker uses (`rolesForLegacyTargets`); the two share their semantics by deliberately mirroring each other so a row written here is indistinguishable from a row migrated from a legacy entry.

Existing rows for the deployment in `deployment_build_env` are cleared up-front with a single `DELETE`, so an update-deploy gets a clean per-(role, env) set every time.

### Backfill

A **River periodic job** (`BuildEnvBackfillWorker`, `RunOnStart: true`, 24h cadence). Per deployment:

- Skip if rows already exist (idempotent — re-runs and crashes are safe).
- Read `deployment_variables` rows for that deployment.
- For each variable, fan out across roles derived from `Targets`:
  - `"agent"` → role `agent`
  - `"interface.<adapter>"` → role `messaging` (one row per variable, regardless of how many adapter targets it has)
  - `"ingestion"` → role `ingestion:<name>` for every ingestion declared in `deployment_spec_json`
  - `"ingestion.<name>"` → role `ingestion:<name>` only
- Write rows under `source='user_var'`, carrying the original `account_var_ref` for prefill correctness.

**No KMS round-trip.** `deployment_variables` already stores secret values as `base64(ciphertext) + nonce`; the backfill base64-decodes the value and writes raw ciphertext bytes with the same nonce into `value_encrypted`. Non-secret values were stored as plaintext `text`; they go in as plaintext bytes with `nil` nonce. Same encryption story end-to-end, no decrypt step needed.

## Migration

1. Land this PR.
2. Atlas creates `deployment_build_env` on each environment. River boots the backfill on first server start; the job drains within minutes for any normal deployment count, then no-ops on subsequent invocations.
3. Verify rows exist for every deployment with user variables before merging the follow-up PR that drops `deployment_variables`. A spot check is enough — failed deployments are logged with their ID.

This PR alone has zero behavior change for application code: no consumer reads from `deployment_build_env` yet.
