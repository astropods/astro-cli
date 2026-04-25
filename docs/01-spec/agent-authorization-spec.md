# Agent Authorization Specification

**Version**: 1.0
**Status**: In progress
**Date**: 2026-04-25

## Overview

Authorization for messaging containers across all interfaces (web, slack). Agent owners declare which users and accounts can talk to a deployed agent; the platform enforces the policy inside the messaging container without requiring the container to know about identity providers.

This complements the existing ALB OIDC (authn) layer documented in `messaging-oidc-auth-spec.md`. OIDC answers "is this a real authenticated user." This spec answers "is this principal allowed to use *this* deployment."

## Terminology

- **Principal** — the raw caller identity carried in the request: a WorkOS user ID for web, a slack user ID for slack. This is what the messaging container forwards to astro-server.
- **Account** — the unit of access for whole-team grants. Comes in two flavors with no schema-level distinction: a *personal account* has one member; an *org-backed account* has many members and is linked to a WorkOS org via `account_organizations` (1:1).
- **Resolution** — translating a principal into the candidates that may match a grant. WorkOS user → that user ID *plus* every account the user is a member of (`account_members`). Slack user → the bot's owning account only.
- **Subject** — the target of a grant. One of:
  - `account` — any member of this account is allowed.
  - `user` — one specific WorkOS user is allowed.
  - `anyone` — anyone hitting this adapter is allowed (a "public" grant). Web-only.
- **Grant** — a row in `deployment_authorization_grants` saying "this subject may use this deployment via this adapter."
- **Adapter** — the messaging interface the grant applies to: `web` or `slack`.

## Goals

1. **Whole-team, per-user, and public policy**: owners can grant to an entire account, to specific users, or open the web adapter to anyone. All three are uniform grants — no separate flags.
2. **Works for restrictive org, additive personal, and public cases**: see Worked scenarios.
3. **Per-adapter granularity**: web and slack auth are configured independently.
4. **Spec-driven, single source of truth**: policy lives in `interfaces.auth` and changes only through the deployment template + deploy flow. No imperative admin endpoints.
5. **Cheap at request time**: short-TTL cache in the messaging container; one DB hit per cache miss.

## Non-Goals

1. **Per-slack-user authorization**: slack identity is opaque; we authorize the bot's owning account. `user` grants are web-only.
2. **Roles inside an agent**: access is binary (allowed/denied). No viewer/admin distinction — meaningless for an agent.
3. **Replacing ALB OIDC**: web requests still flow through ALB OIDC for authn first.
4. **Anonymous access**: every request must carry a resolvable identity.
5. **Inviting non-users by email**: a user grant requires the user already exists in WorkOS. Email invitations are a separate platform flow.

## Identity model

The principal is a raw identity (WorkOS user ID or slack user ID). Astro-server resolves it to a set of candidates before matching against grants.

- **Web (ALB OIDC + WorkOS)**: ALB injects `x-amzn-oidc-identity` (WorkOS user ID). The messaging container forwards this as the principal. Astro-server resolves user → the user ID itself plus every account the user is a member of via `account_members`.
- **Slack**: the slack bot is installed by an account. The messaging container forwards the slack user ID as the principal, but slack identity is opaque — we cannot independently authenticate it. Astro-server resolves the call to the bot's owning account (from the deploy token) only. The slack user ID is recorded for logging.
- **Future**: API keys would resolve to a single account directly.

## Authorization check

A request is allowed **iff a matching grant exists**. There is no default-allow fallback and no implicit owner access. If you want someone (or everyone in some account, or anyone) to access the agent, there must be a grant for them.

The check happens in two places by design — the messaging container short-circuits the easy case locally; the server is the authority for everything else.

### Container-side fast path (public traffic)

The deploy token carries `anyone_adapters` — the list of adapters that have an `anyone` grant *as of token issuance*. The messaging container reads this list when starting up and, for any request on a listed adapter, allows immediately without an authorize call.

This is safe because:
- Grants only change via redeploy (no imperative API), and every redeploy reissues the token in lockstep.
- The list is signed in the token, so a client cannot forge it.
- The server-side check still enforces the same rule, so a bug or stale client doesn't widen access — at worst it issues a redundant authorize call.

For anonymous public deployments (`anyone` grant + ALB OIDC off), this means the messaging container never needs to call back at all. No identity required, no cache miss penalty.

### Server-side authoritative check

For every other request, the messaging container calls `/deployments/authorize` with the deploy token + identity. The server runs:

Inputs:
- `deployment_id` (from the signed deploy token — claim is `sub`)
- `identity_type` (`user` | `slack`), `identity_id`, `adapter` (query params)

Flow:

1. **`anyone` short-circuit** — if a row exists with `subject_type='anyone'` for `(deployment_id, adapter)`, allow without resolving the principal. Defense in depth: even if a client doesn't honor `anyone_adapters` from the token, the server still gets it right.
2. **Principal resolution** — produce a candidate set:
   - `user`: `{ user_id, account_id_1, account_id_2, ... }` (the user plus all their accounts).
   - `slack`: `{ deployment_account_id }` — looked up from the `deployments` row by `deployment_id`. Resolved at request time so ownership transfers don't require token re-issuance.
3. **Grant lookup** — single SQL query against `deployment_authorization_grants` filtered by `deployment_id` + `adapter`, matching:
   - `subject_type='user' AND subject_id ∈ candidates`, **or**
   - `subject_type='account' AND subject_id ∈ candidates`
   Allow if any row hits, deny otherwise.

The deploy template's prefill (below) ensures the deployer ends up in the grants list, so a fresh deployment isn't dead-on-arrival in the common path.

The messaging container caches the boolean per `(deployment_id, identity, adapter)` for ~60s. Public-adapter requests skip the cache entirely (the fast path doesn't go through it).

## Configuration surface

Authorization is configured exclusively through the `interfaces.auth` block in the deployment spec. Editing always goes through the deployment template flow followed by a redeploy — there is no imperative admin endpoint that mutates policy or grants out-of-band. This keeps the spec the single source of truth and makes audit/rollback trivial (every policy change is a deploy with its own audit trail).

On deploy, astro-server atomically replaces all rows in `deployment_authorization_grants` for the deployment with the spec's grants list.

The grants table is the runtime source of truth used by the authorize endpoint, but it's never written to except by the deploy path. The template-prefill endpoint reads from it so the UI always reflects live state.

YAML shape:

```yaml
interfaces:
  auth:
    web:
      type: oidc          # existing — controls ALB-level authn
    grants:
      - account_id: <uuid>      # any member of this account, web adapter
        adapter: web
      - user_id: <workos_user_id>  # this specific user only, web adapter
        adapter: web
      - account_id: <uuid>      # slack: account-only (no user_id allowed)
        adapter: slack
      - anyone: true            # web only — opens the adapter to anyone
        adapter: web
```

A grant must specify exactly one of `account_id`, `user_id`, or `anyone: true`. `user_id` and `anyone` grants must use `adapter: web` — slack rejects them at deploy.

**Public deployment composition.** A truly anonymous deployment requires both an `anyone` grant *and* `web.type` left unset (so ALB OIDC isn't gating the ingress). With `web.type=oidc` plus an `anyone` grant, any authenticated WorkOS user gets in but anonymous traffic still doesn't — ALB rejects it before authz runs. The two layers compose; we don't try to hide one behind the other.

## Defaults and prefill

The deploy template returned for a brand-new deployment ships with sensible grants pre-populated, so a default deploy isn't dead-on-arrival. The user sees these in the UI/CLI before submitting and can edit them.

Prefill rules on a fresh deploy:
- One `user` grant for the deployer's WorkOS user ID, `adapter: web`
- One `account` grant for the deployment's owner account, `adapter: slack` (only if slack is in `interfaces.adapters`)

Prefill is a starting point, not enforcement. Removing the prefilled grants before deploying is allowed; the resulting deployment will deny everyone (which is a valid configuration if the deployer plans to add specific grants and never use the agent themselves).

For redeploys, prefill comes from the live grants table, not from these rules — the user sees what's currently in effect.

## Token mechanism

A signed deploy token proves "I am this deployment" and carries the small set of state that's worth caching client-side because changing it requires a redeploy anyway. Anything else (owning account, account-scoped grants, user grants) is looked up server-side by `deployment_id`.

- HMAC-SHA256 JWT signed with `DEPLOY_TOKEN_SECRET` (server-level config; defaults to `astro-dev-secret` in dev so local works without setup).
- Issued at K8s spec-apply time; injected as the `ASTRO_DEPLOY_TOKEN` env var on the messaging sidecar.
- Claims:
  - `sub` = `deployment_id`
  - `iss` = `astro-server`
  - `anyone_adapters` = list of adapters with an `anyone` grant at issuance (e.g. `["web"]` or `[]`). Lets the container short-circuit public traffic without calling the server.
- No `account_id` claim — owning-account is mutable, so we look it up at request time instead of carrying stale state in the token.
- No expiry today (rotation = redeploy). Every grant change requires a redeploy, so the token's `anyone_adapters` cannot drift from the live grants table.
- `RequireDeployToken` middleware validates the Bearer token on `/deployments/authorize` and injects `deployment_id` into the gin context. `anyone_adapters` is consumed by the messaging container at startup, not by the server.

## Schema

```
deployment_authorization_grants(
  id            uuid PK,
  deployment_id FK→deployments,
  subject_type  varchar CHECK IN ('account', 'user', 'anyone'),
  subject_id    varchar NOT NULL,           -- accounts.id (uuid as text) | workos_user_id | '' for anyone
  adapter       varchar CHECK IN ('web', 'slack'),
  created_at, updated_at,
  UNIQUE (deployment_id, subject_type, subject_id, adapter),
  CHECK (subject_type IN ('user', 'anyone') = false OR adapter = 'web'),  -- user/anyone grants are web-only
  CHECK (subject_type <> 'anyone' OR subject_id = '')                     -- anyone uses empty subject_id
)
```

`subject_id` has no FK because it's polymorphic. For `anyone` grants the column is the empty string (not NULL) so the unique index catches duplicates. Cascade only on `deployment_id`. The unique index supports the runtime lookup directly.

There is no separate access-policy table — a request is allowed iff a row exists for it.

## Worked scenarios

**1. Personal account deploy, default**
- Jane deploys her agent on her personal account.
- Prefill: `user_id=Jane, adapter=web`. (Slack isn't enabled.)
- Jane talks via web. No one else can.

**2. Personal account, share with one outside user**
- Jane wants Bob (different personal account) to use her agent.
- She adds `user_id=Bob, adapter=web` to grants.
- Bob talks via web. Other users still denied.

**3. Org-backed account, all members**
- Acme deploys an internal agent open to the whole company.
- Owner sets `account_id=Acme, adapter=web` (and `adapter=slack` if applicable).
- Every Acme member can talk via the granted adapters.

**4. Org-backed account, restricted to specific members**
- Acme deploys an HR agent that only Alice and Bob (Acme members) should use.
- Owner does *not* add an account grant. Adds `user_id=Alice, adapter=web` and `user_id=Bob, adapter=web`.
- Other Acme members are denied.

**5. Org-backed account with slack, restricted on web**
- Acme deploys with slack enabled (any Acme member should use the bot) but web restricted.
- Grants: `account_id=Acme, adapter=slack`, `user_id=Alice, adapter=web`, `user_id=Bob, adapter=web`.
- Slack: any Acme member. Web: Alice and Bob only.

**6. Public web agent (any authenticated user)**
- Acme deploys a help-desk agent open to any authenticated WorkOS user.
- `interfaces.auth.web.type=oidc` (ALB OIDC stays on), grants include `anyone: true, adapter: web`.
- Anyone who passes WorkOS login can talk via web. Slack is unaffected (separate grants).

**7. Truly public agent (anonymous web)**
- Acme deploys a public marketing demo with no login.
- `interfaces.auth.web.type` unset (ALB OIDC off), grants include `anyone: true, adapter: web`.
- Anonymous traffic is allowed; the messaging container's authorize call short-circuits on the `anyone` grant without needing an identity.

## Test cases

The implementation must pass every case in this list. Cases are grouped by surface area; IDs are stable so individual cases can be referenced.

### A. Authorization at request time

| ID  | Scenario                                                                                                              | Expected                                                                                                |
| --- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| A1  | Grant `(account, Acme, web)`; Alice is a member of Acme; web request from Alice                                       | allowed                                                                                                 |
| A2  | Grant `(account, Acme, web)`; Bob is not a member of Acme; web request from Bob                                       | denied                                                                                                  |
| A3  | Grant `(user, Alice, web)`; web request from Alice                                                                    | allowed                                                                                                 |
| A4  | Grant `(user, Alice, web)`; web request from Bob (same account as Alice)                                              | denied                                                                                                  |
| A5  | A `user` grant exists with `adapter=slack` (DB corruption — validation should prevent); slack request                 | does not match; treat as no grant                                                                       |
| A6  | Grant `(anyone, '', web)`; token's `anyone_adapters` includes `web`; authenticated web request                        | allowed by container without server call                                                                |
| A7  | Grant `(anyone, '', web)`; ALB OIDC off; anonymous web request                                                        | allowed by container without server call                                                                |
| A8  | Misbehaving client ignores `anyone_adapters` and calls authorize anyway; `anyone` grant exists                        | server still allows (defense in depth)                                                                  |
| A9  | `anyone` grant for slack (should not exist); slack request                                                            | does not match; rejected by validation upstream                                                         |
| A10 | Deployment has no grants; any request                                                                                 | denied — deployer is *not* implicitly allowed; token's `anyone_adapters` is `[]`                        |
| A11 | Grants `(user, Alice, web)` and `(account, Acme, web)`; web request from Bob (in Acme)                                | allowed via account grant                                                                               |
| A12 | Alice is in accounts X and Y; grant `(account, Y, web)`; web request from Alice                                       | allowed                                                                                                 |
| A13 | Deployment owned by D; grant `(account, D, slack)`; slack request                                                     | allowed                                                                                                 |
| A14 | Deployment owned by D; no slack grant; slack request                                                                  | denied                                                                                                  |
| A15 | Grant `(account, Acme, web)`; slack request from Acme member                                                          | denied (adapter mismatch)                                                                               |
| A16 | Grant `(account, Acme, slack)`; web request from Acme member                                                          | denied (adapter mismatch)                                                                               |
| A17 | Token's `sub` doesn't match any deployment                                                                            | 401/404 (not silent deny)                                                                               |
| A18 | Authorize endpoint called with no Bearer token                                                                        | 401                                                                                                     |
| A19 | Token signature invalid / wrong issuer / wrong secret                                                                 | 401                                                                                                     |
| A20 | Authorize call with empty `identity_type`/`identity_id`; no `anyone` grant                                            | denied                                                                                                  |
| A21 | Deployment row's `account_id` updated; existing token used                                                            | token still validates; slack resolution uses the new account_id                                         |
| A22 | Deploy emits an `anyone` grant for adapter X                                                                          | issued token's `anyone_adapters` includes X exactly; no divergence allowed                              |

### B. Principal resolution

| ID  | Input                                                | Candidate set                                                              |
| --- | ---------------------------------------------------- | -------------------------------------------------------------------------- |
| B1  | WorkOS user, member of one account                   | `{user_id, account_id}`                                                    |
| B2  | WorkOS user, member of N accounts                    | `{user_id, account_id_1, ..., account_id_N}`                               |
| B3  | WorkOS user with no account memberships              | `{user_id}` (account grants will not match)                                |
| B4  | Slack identity                                       | `{deployment_account_id}` from `deployments` row; slack user_id logged only |
| B5  | Empty identity                                       | resolution skipped; only `anyone` short-circuit can pass                   |

### C. Spec validation (deploy-time)

| ID  | Spec input                                                          | Expected     |
| --- | ------------------------------------------------------------------- | ------------ |
| C1  | Grant with both `account_id` and `user_id`                          | reject       |
| C2  | Grant with both `account_id` and `anyone`                           | reject       |
| C3  | Grant with both `user_id` and `anyone`                              | reject       |
| C4  | Grant with no subject set                                           | reject       |
| C5  | `user_id` grant with `adapter: slack`                               | reject (web-only) |
| C6  | `anyone` grant with `adapter: slack`                                | reject (web-only) |
| C7  | Unknown adapter value                                               | reject       |
| C8  | Two identical `(subject, adapter)` entries in one spec              | reject (dup) |
| C9  | Malformed `account_id` (not a UUID)                                 | reject       |
| C10 | `anyone: false`                                                     | reject — omit the line instead |
| C11 | `interfaces.auth` block omitted entirely                            | existing grants preserved (no-op) |
| C12 | `interfaces.auth` present, `grants: []`                             | all grants deleted |

### D. Schema constraints (DB-level)

| ID  | Insert / op                                                         | Expected            |
| --- | ------------------------------------------------------------------- | ------------------- |
| D1  | Row with `subject_type` not in {account, user, anyone}              | CHECK violation     |
| D2  | Row with `(user, *, slack)`                                         | CHECK violation     |
| D3  | Row with `(anyone, '', slack)`                                      | CHECK violation     |
| D4  | Row with `(anyone, 'non-empty', web)`                               | CHECK violation     |
| D5  | Duplicate `(deployment_id, subject_type, subject_id, adapter)`      | unique violation    |
| D6  | Delete deployment row                                               | grants cascade-delete |

### E. Deploy flow

| ID  | Scenario                                                                                  | Expected                                                                                |
| --- | ----------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| E1  | Fresh deploy, no `interfaces.auth` block                                                  | prefill: `(user, deployer, web)` + `(account, owner, slack)` if slack enabled           |
| E2  | Fresh deploy, explicit auth block                                                         | user's block is authoritative; defaults not added on top                                |
| E3  | Deploy fails validation mid-write                                                         | atomic — neither deployment nor grants persist                                          |
| E4  | Redeploy via template with `deployment_id`                                                | prefill returns the live grants, not fresh-deploy defaults                              |
| E5  | Existing grants `[A, B]`; spec sends `[A, C]`                                             | result is `[A, C]`; B removed                                                           |
| E6  | Existing grants `[A, B]`; spec sends `auth: { grants: [] }`                               | result is `[]`                                                                          |
| E7  | Existing grants `[A, B]`; spec omits the auth block                                       | result is `[A, B]` unchanged                                                            |

### F. Configuration surface

| ID  | Surface                                                                                   | Expected                                                                                |
| --- | ----------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| F1  | `GET/PUT/DELETE /api/v1/deployments/:id/authorization*`                                   | 404 — endpoint does not exist                                                           |
| F2  | `/deployments/authorize` called by anything other than messaging container with token     | 401 (no valid deploy token)                                                             |
| F3  | astro-client / astro-cli call any non-template path to mutate grants                      | no such path exists                                                                     |

### G. Token mechanism

| ID  | Scenario                                                  | Expected                                                                |
| --- | --------------------------------------------------------- | ----------------------------------------------------------------------- |
| G1  | Sign with secret S, verify with S                         | passes; claims roundtrip                                                |
| G2  | Sign with S, verify with S′                               | reject                                                                  |
| G3  | Token with `iss` ≠ `astro-server`                         | reject                                                                  |
| G4  | Token missing `sub` claim                                 | reject                                                                  |
| G5  | Token contains stale `account_id` claim                   | ignored — server uses deployment row                                    |
| G6  | `DEPLOY_TOKEN_SECRET` unset                               | falls back to `astro-dev-secret`; local dev works                       |
| G7  | Deploy with `anyone` grant for web                        | issued token has `anyone_adapters: ["web"]`; without it, `[]`           |
| G8  | Token claim edited (signature stale)                      | reject; cannot widen access by editing                                  |

### H. Cache (messaging container)

| ID  | Scenario                                                                                  | Expected                                |
| --- | ----------------------------------------------------------------------------------------- | --------------------------------------- |
| H1  | Two requests within TTL for same `(deployment_id, identity, adapter)`                     | second skips network call               |
| H2  | Request after TTL                                                                         | hits server again                       |
| H3  | Different identity / adapter / deployment                                                 | separate cache entries                  |
| H4  | Denied result                                                                             | cached for full TTL (no thrash)         |
| H5  | Adapter listed in `anyone_adapters`                                                       | bypasses cache entirely; allowed inline |

### I. Cross-account scenarios

| ID  | Scenario                                                                              | Expected                              |
| --- | ------------------------------------------------------------------------------------- | ------------------------------------- |
| I1  | Alice's personal deployment grants `(user, BobUserID, web)`; Bob has his own account  | Bob allowed via web                   |
| I2  | Acme's deployment grants `(user, BobUserID, web)`; Bob is not in Acme                 | Bob allowed via web                   |
| I3  | Grants `(anyone, '', web)` and `(account, Acme, slack)`                               | anyone on web; only Acme on slack     |

### J. Observability / audit

| ID  | Scenario                                              | Expected                                                            |
| --- | ----------------------------------------------------- | ------------------------------------------------------------------- |
| J1  | Deploy changes auth                                   | audit event recorded; no silent grant changes                       |
| J2  | Authorize call denies                                 | server logs `(deployment_id, identity, adapter, reason)` for debug  |

## Components and ownership

- **astro-server** owns the schema, the admin API, the authorize endpoint, the deploy token signer, and the spec → DB writer.
- **messaging container** holds the deploy token, calls the authorize endpoint, and caches the result.
- **astro-client (UI)** edits `interfaces.auth` via the deployment template flow; reads back live state from the prefill response.
- **astro-cli** passes `interfaces.auth` through verbatim — no special handling.

## Implementation status

**Done** (account-only, with implicit owner, includes admin endpoints to be removed):
- Deploy token sign/verify (`internal/deploytoken`)
- `RequireDeployToken` middleware (`internal/middleware`)
- Authorization store, account-keyed (`internal/authorizationstore`)
- Spec block with `{account_id, adapter}` grants (`packages/astro-spec`)
- Deploy-time apply + template prefill from DB (`handlers/deploy.go`)
- Token injection into messaging sidecar (`internal/k8s/spec_applier.go`, `deployment.go`)

**Pending**:
1. **Remove imperative admin endpoints** in `handlers/authorization.go` and their routes in `main.go`. Keep only the messaging-facing `CheckDeploymentAuthorization` (the `/deployments/authorize` endpoint behind `RequireDeployToken`).
2. **Drop `deployment_access_policy` table** and all `default_allow` plumbing in store, handlers, and spec.
3. Token: drop the `account_id` claim; add `anyone_adapters` claim populated from the grants table at sign time. `RequireDeployToken` injects only `deployment_id`.
4. Schema: add `subject_type` + `subject_id` columns; allowed values `account`/`user`/`anyone`; add the web-only CHECK for `user`/`anyone`.
5. Store: query all three subject types in one lookup; short-circuit on `anyone` before principal resolution; resolve `account_id` for slack from the `deployments` row.
6. Spec: grant becomes `{account_id | user_id | anyone, adapter}`; validation enforces exactly one and the web-only constraint for `user`/`anyone`.
7. Authorize handler: short-circuit on `anyone`; otherwise include `user_id` in the candidate set for web and resolve user → accounts; look up `account_id` for slack from the deployment row. Accept empty identity inputs (anonymous) and rely on the `anyone` short-circuit to allow them.
8. Deploy template prefill: auto-populate the deployer's user grant (web) and owner account grant (slack, if enabled) on fresh deployments; on redeploy, prefill from the live DB.
9. Astro-client UI: support adding both account and user grants in the deployment template flow; surface the org-backed-account distinction so users understand grant blast radius.
10. Messaging client: parse `anyone_adapters` from the deploy token at startup; short-circuit matching adapter requests without calling the server. For all other requests, short-TTL cache for the boolean response.

## Open questions

1. **Cache invalidation** — grant changes (via redeploy) are not visible to the messaging container until the local cache TTL expires. Acceptable for v1. Options if we need explicit revoke later: shorter TTL, or a versioned token bumped on auth change.
2. **Validating user IDs at deploy time** — should we check the WorkOS user actually exists when applying a `user` grant? Cheaper to catch typos at deploy than to debug silent denies; recommend yes if a fast lookup exists.
3. **UI surfacing of org-backed accounts** — when adding an account grant the picker should distinguish "Acme Corp (org, 50 members)" from "Jane Doe (personal)" so the user understands blast radius.
4. **`anyone` + slack** — currently rejected (slack identity is the bot's owning account, so "anyone" is meaningless). Revisit if a cross-workspace slack model emerges.
5. **Empty-identity authorize calls** — the server still accepts and handles them (for misbehaving clients), but the documented happy path for anonymous public traffic is the container's `anyone_adapters` short-circuit, not a server round-trip.
