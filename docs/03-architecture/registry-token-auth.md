# Registry Token Authentication

How `astro-registry` authenticates Docker push/pull operations using the
Docker Registry v2 token-auth flow, decoupling registry-scope tokens from
WorkOS access-token TTL.

## Problem

`astro-cli push` resolves the user's WorkOS access token once and hands it
to the Docker daemon as `RegistryAuth`. The daemon reuses that single
bearer for every blob `PUT` and the final manifest `PUT`. `astro-registry`
revalidates the JWT's `exp` on every `/v2/*` request.

When push duration exceeds the WorkOS access-token TTL (default ~5 min),
the manifest `PUT` — always last — is rejected `401`. Blob layers already
on the wire succeed; the push then fails at the final step. Refreshing on
the CLI side does not help: the daemon holds the token for the full push
and `dockerPushWithRetry` only retries 5xx.

## Background: Docker Registry v2 token auth

The Distribution spec defines a two-token model:

- The IdP token (here: WorkOS access token) authenticates the *user* to the
  registry's token endpoint.
- The registry-scope token authenticates *requests against the registry*.
  It is signed by the registry, scoped to specific repositories and
  actions, and has its own TTL.

Reference: `distribution/distribution/docs/spec/auth/{token,jwt,scope}.md`.

The flow is the standard `WWW-Authenticate: Bearer realm=…` negotiation:
client hits `/v2/*` without a registry token, gets `401` with a realm
pointer, calls the realm to exchange IdP credentials for a registry token,
then retries `/v2/*` with the registry token. Docker, containerd, BuildKit,
skopeo, crane, and the major cloud registries (ECR/GCR/GHCR) all speak it.

## Design

```
┌──────┐  ┌────────┐  ┌──────────┐  ┌────────┐  ┌──────┐  ┌─────┐
│ cli  │  │ daemon │  │ registry │  │ WorkOS │  │ ECR  │  │ DB  │
└──┬───┘  └───┬────┘  └────┬─────┘  └───┬────┘  └──┬───┘  └──┬──┘
   │ push    │            │            │          │         │
   │────────▶│ PUT blob1 (no auth)     │          │         │
   │         │───────────▶│ 401 WWW-Authenticate: │         │
   │         │            │   realm=…/token,      │         │
   │         │            │   service=astro-registry,       │
   │         │            │   scope=repo:ns/img:push,pull   │
   │         │◀───────────│            │          │         │
   │         │ GET /token?service=…&scope=…       │         │
   │         │ Basic("token", WorkOS)             │         │
   │         │───────────▶│ verify W ─▶│ ok       │         │
   │         │            │ check scope ─── membership ────▶│
   │         │            │            │          │ ok      │
   │         │            │ mint R = HS256{iss=astro-registry,
   │         │            │   sub, access=[…], exp=+1h}     │
   │         │ 200 {token: R, expires_in: 3600}             │
   │         │◀───────────│            │          │         │
   │         │ PUT blob1 Bearer R                 │         │
   │         │───────────▶│ verify R (sig+scope, local)     │
   │         │ 200/202    │── proxy ─▶│           │         │
   │         │◀───────────│            │          │         │
   │       … many blobs, each verifies R locally …          │
   │         │ PUT manifest Bearer R              │  (T+10m)│
   │         │───────────▶│ verify R ✓ │          │         │
   │         │ 201        │── proxy ─▶│           │         │
   │ ✓ OK    │◀───────────│            │          │         │
```

### Components

| Component | Responsibility |
|---|---|
| `internal/auth/registry_token.go` | Mint + verify registry-scope JWT. HS256 with `REGISTRY_TOKEN_SECRET`. |
| `handlers/token.go` | `GET /token` — IdP-authenticated scope grant endpoint. |
| `internal/middleware/auth.go` | `RequireAuth` accepts WorkOS *or* registry tokens; emits `WWW-Authenticate` on 401. |
| `modules/astro-cli/cmd/push_streaming.go` | Sends `Username`/`Password` (not `RegistryToken`) so the daemon honors the realm flow. |

### Token format

Registry token, HS256 signed:

| Claim | Value |
|---|---|
| `iss` | `"astro-registry"` |
| `sub` | WorkOS user ID |
| `aud` | `"astro-registry"` (service name) |
| `iat` / `nbf` / `exp` | issued, not-before, +1h (default) |
| `jti` | random — for log correlation |
| `access` | `[{type:"repository", name:"<ns>/<image>", actions:["pull","push"]}]` |

WorkOS tokens are RS256 with `iss=https://api.workos.com/...`. Registry
tokens are HS256 with `iss=astro-registry`. The middleware parses the
header/iss unverified, then routes to the correct verifier.

### Scope grammar

Per spec: `repository:<name>:<actions>` where actions ∈ {`pull`, `push`,
`delete`}. Multiple resources separated by spaces.

The token endpoint:

1. Parses each `scope` query param.
2. For each `repository:<ns>/<image>:<actions>`, runs the existing
   membership check on `<ns>` (resolves to account UUID), and for org
   accounts the existing permission check (`agents:read` for `pull`,
   `agents:write` for `push`).
3. Drops actions the user is not entitled to (per spec: server returns the
   intersection, never an error). If the resulting set is empty for every
   scope, the token is still issued but with `access=[]` — the registry
   then rejects the actual `/v2/*` request `403`.

### Scope enforcement on `/v2/*`

When a registry token is presented, the middleware:

1. Verifies signature + `exp`.
2. Extracts `<ns>/<image>` and required action from the request:
   - `GET`/`HEAD` → `pull`
   - `PUT`/`POST`/`PATCH`/`DELETE` → `push` (push implies pull)
3. Checks the token's `access` array contains a matching repository with
   the required action. Mismatch → `403`.

Membership and permission checks happen **only at token issuance**, not
per `/v2/*` request. The DB and IdP are touched at most once per push.

### Realm URL

Configured via `REGISTRY_TOKEN_REALM` (e.g. `https://registry.astro.example.com/token`).
Required at boot — the previous request-host derivation silently advertised
unreachable URLs behind TLS-terminating ingress, so it's gone.

### CLI change

Today:

```go
registry.AuthConfig{ RegistryToken: workosToken }   // daemon: Bearer on every request
```

After:

```go
registry.AuthConfig{ Username: "token", Password: workosToken }  // daemon: Basic at /token, Bearer R thereafter
```

`RegistryToken` short-circuits the daemon's auth negotiation and forces it
to ignore `WWW-Authenticate`. `Username`/`Password` is what triggers the
realm exchange. `"token"` is a placeholder username; the token endpoint
ignores it and reads the WorkOS bearer from the password slot.

## Trust model

- **Signing key.** Single shared secret (`REGISTRY_TOKEN_SECRET`, 32+ random
  bytes) loaded at boot. Same binary signs and verifies, so HS256 is
  appropriate. If the token endpoint is ever split into a separate service
  (or external clients need to verify), migrate to RS256 with a JWKS
  endpoint.
- **Secret rotation.** Restart with a new secret invalidates all
  outstanding registry tokens — clients re-exchange on the next 401.
  Short outage window acceptable; no graceful overlap implemented.
- **Token lifetime.** 1h default. Covers any realistic push (~2× P99) without
  leaning on the daemon's mid-push 401-refresh path as the happy case.
  Tunable via `REGISTRY_TOKEN_TTL` if observed pushes exceed this. Leak
  blast radius is narrow (single repo, push/pull only — no escalation).
  Revocation is not supported (industry standard for short-TTL bearers).
- **No replay protection.** `jti` is for logging only. Tokens are bearer
  credentials — assume HTTPS-only transport.

## Failure modes

| Failure | Behavior |
|---|---|
| WorkOS down at `/token` | `502` — push fails fast at start instead of mid-stream. |
| WorkOS down mid-push | No effect. Registry token already minted; verification is local. |
| Registry secret rotated mid-push | Next `/v2/*` request 401s, daemon transparently re-exchanges at `/token`, push continues. |
| Registry token expires mid-push | Same — daemon re-exchanges. (Should not happen with 1h TTL for any realistic push.) |
| User loses access mid-push | Push completes for the duration of the existing token (≤1h). Acceptable: WorkOS revocation is itself delayed by the access-token TTL. |
| Malformed scope query param | `400` from `/token`. |

## Spec compliance

| Spec requirement | Status |
|---|---|
| `WWW-Authenticate: Bearer realm=…,service=…,scope=…` on 401 | ✓ |
| `GET <realm>?service=…&scope=…` | ✓ |
| HTTP Basic at the realm | ✓ (password = WorkOS bearer) |
| Response `{"token", "expires_in", "issued_at"}` | ✓ |
| Server-signed JWT with `access` claim | ✓ |
| Scope intersection (drop unauthorized actions, don't error) | ✓ |
| `pull` implied by `push` | ✓ |

Deviations from `docker/distribution`'s reference implementation:

- HS256 instead of RS256 with x5c chain — single signer/verifier.
- Token server folded into `astro-registry` instead of a separate service —
  reuses existing membership/permission logic.

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `REGISTRY_TOKEN_SECRET` | (required) | HMAC key for signing registry tokens. |
| `REGISTRY_TOKEN_ISSUER` | `astro-registry` | `iss` claim. |
| `REGISTRY_TOKEN_TTL` | `1h` | Token lifetime. Bump if pushes routinely exceed this. |
| `REGISTRY_TOKEN_REALM` | (required) | Public URL of `/token` advertised to clients. |

## Compatibility & rollout

- **WorkOS-bearer clients** (dashboard, non-Docker pulls) — fully compatible.
  `RequireAuth` peeks at `iss`, routes WorkOS-issued JWTs to the existing
  validator. No behavior change.
- **Old astro-cli binaries** still send `RegistryToken: <workos_jwt>`. Docker
  forwards that as `Authorization: Bearer …`, the registry routes to the
  WorkOS path. They still hit the original long-push bug — no regression,
  no fix.
- **New astro-cli against old astro-registry — forward-incompatible.** The
  new CLI sends `Username`/`Password` and relies on the daemon following
  `WWW-Authenticate: Bearer realm=…/token`. Old registries don't emit the
  realm; the daemon falls back to Basic on `/v2/*`, which the old registry
  rejects.

Safe rollout order:

1. Set `REGISTRY_TOKEN_SECRET` on the registry deployment (boot validates it).
2. Roll out the new astro-registry image. Old CLIs keep working.
3. Ship the new astro-cli to users.

## Side effects

- WorkOS validation reduced from once-per-request to once-per-push
  (~hundreds of HTTPS calls → 1). Reduces coupling between push duration
  and IdP availability.
- DB membership query reduced from once-per-request to once-per-push for
  the push path. Existing per-request check remains for WorkOS-bearer
  clients (UI, non-Docker pulls).
