# Per-deployment authorization

## Summary

Agent owners can now declare who is allowed to talk to a deployed agent — by account, by user, or open to anyone — independently per adapter (`web`, `slack`). Previously the messaging container had no per-deployment access control beyond ALB OIDC authentication, so any authenticated WorkOS user could reach any agent. This adds the missing authorization layer enforced inside the messaging container with an authoritative server-side check.

This complements the existing OIDC authn layer: OIDC answers "is this a real user," and grants answer "is this principal allowed to use *this* deployment."

## Design

**Spec is the single source of truth.** Policy lives in `interfaces.auth.grants` in the deployment spec and changes only through the deploy flow. There is no imperative admin API that mutates grants out-of-band; every policy change is a redeploy with its own audit trail. On deploy, astro-server atomically replaces all rows in `deployment_authorization_grants` for the deployment with the spec's grants list.

**Three subject types, grants nested per adapter.** Grants live under the adapter they apply to (`web.grants` or `slack.grants`); the adapter is implied by where the grant sits, not carried as a field on each grant. Each grant picks one of `account_id`, `user_id`, or `anyone: true`:

```yaml
interfaces:
  auth:
    web:
      type: oidc                       # ALB-level authn (existing)
      grants:
        - account_id: <uuid>           # any member of this account
        - user_id: <workos_user_id>    # one specific user (web only)
        - anyone: true                 # public (web only)
    slack:
      grants:
        - account_id: <uuid>           # slack is account-only
```

`user_id` and `anyone` are web-only — slack identity is opaque, so the platform only authorizes the bot's owning account. Validation rejects them under `slack.grants` at deploy time, and schema-level CHECK constraints enforce the same invariant in the storage layer.

**Two-layer enforcement.** The messaging container short-circuits the easy case; the server is the authority for everything else:

- **Container fast path.** The signed deploy token now carries `anyone_adapters` — the list of adapters with an `anyone` grant at issuance. The container reads this at startup and allows requests on listed adapters without calling back. Safe because grants only change via redeploy and tokens reissue in lockstep.
- **Server-side `/deployments/authorize`.** For every other request, the container forwards the principal (WorkOS user ID for web, slack user ID for slack) and the adapter. The server resolves the principal to a candidate set (user → user ID + all their accounts; slack → deployment's owning account) and runs a single SQL match against `deployment_authorization_grants`. Allow iff a row matches.

**No-grants fallback.** A deployment with zero grants falls back to owner-account access (any member of the owning account is allowed). This keeps pre-rollout deployments and explicitly-cleared deployments working without instant lockout. The fallback turns off the moment any grant is added.

**Prefill, not enforcement.** Fresh deployments get a starter `user` grant for the deployer (web) and an `account` grant for the owner account (slack, if enabled). Users see and can edit these before submitting. Redeploys prefill from live grants so the UI reflects current state.

**Token rename and identity injection.** The deploy token env var is now `ASTRO_AUTHZ_TOKEN` (was `ASTRO_DEPLOY_TOKEN`) and is injected into the agent container too — not just the messaging sidecar — so agent code can identify itself to platform APIs. Per-store credential keys avoid `envFrom` collisions when multiple secret stores share names. `AGENT_URL`/`AGENT_HOST` renamed to `ASTRO_AGENT_URL`/`ASTRO_AGENT_HOST` for namespacing consistency.

**Schema.** New `deployment_authorization_grants` table:

```
(deployment_id, subject_type, subject_id, adapter)  -- unique
subject_type IN ('account','user','anyone')
adapter IN ('web','slack')
CHECK: user/anyone grants require adapter='web'
CHECK: anyone grants require subject_id=''
```

`subject_id` is polymorphic (account UUID as text, WorkOS user ID, or empty string for `anyone`) so there's no FK on it. The unique index doubles as the runtime lookup index.

## Migration

No action required for existing deployments. Deployments without grants continue to work via the owner-account fallback. Owners who want explicit per-user, per-account, or public access add `interfaces.auth.grants` to their spec and redeploy.

Operators must set `DEPLOY_TOKEN_SECRET` in production (defaults to `astro-dev-secret` for local dev). Agents and messaging sidecars consuming the deploy token must read `ASTRO_AUTHZ_TOKEN` (renamed from `ASTRO_DEPLOY_TOKEN`); existing deployments pick up the new env var automatically on redeploy.
