## Summary

`astro-cli push` failed with `401 Unauthorized` on the final manifest `PUT`
when the push outlived the WorkOS access-token TTL (~5 min). Blob layers
uploaded fine, then the manifest was rejected because the daemon was
reusing one short-lived WorkOS bearer for every request. This change
implements the standard Docker Registry v2 token-auth flow and decouples
push duration from IdP-token lifetime.

See [docs/03-architecture/registry-token-auth.md](../03-architecture/registry-token-auth.md)
for the full design.

## Design

`astro-registry` now mints registry-scope tokens at a `/token` endpoint and
verifies them on `/v2/*` requests. The WorkOS access token authenticates
the user once, at `/token`, and is exchanged for a registry-signed JWT
that lives for the duration of the push.

```
401 /v2/...           ─ WWW-Authenticate: Bearer realm=…/token,service=…,scope=…
GET /token (Basic W)  ─ verify W, check membership, mint R = HS256{access:[…], exp:+1h}
PUT /v2/... Bearer R  ─ verify signature + scope locally, proxy to ECR
```

**Token format.** HS256 with a single shared secret. Single signer,
single verifier — RS256/JWKS would be overkill. Claims include the
Distribution-spec `access` array plus an Astro-specific `account_id` field
that lets the proxy do its ECR path rewrite without a per-request DB
lookup.

**Routing two token types.** `RequireAuth` peeks the unverified `iss` and
sends WorkOS tokens (existing UI / non-Docker pulls) to the WorkOS
validator and registry-issued tokens (`iss=astro-registry`) to the new
verifier. Registry tokens additionally have their `access` claim enforced
against the request path/method (push implies pull per spec).

**CLI change.** The Docker daemon honors the `WWW-Authenticate` realm flow
only when given `Username`/`Password`. With the previous `RegistryToken`
field, the daemon short-circuited the negotiation and reused the WorkOS
bearer on every request — that was the bug. The CLI now sends
`Username: "token"` and the WorkOS bearer in `Password`; the token endpoint
ignores the username and reads the bearer from the password slot.

**TTL.** 1h. Covers any realistic push (~2× P99) without depending on the
daemon's mid-push refresh path. Tunable via `REGISTRY_TOKEN_TTL`.

**Scope side effect.** Membership and permission checks now run once per
push (at `/token`) instead of once per layer. WorkOS validation drops
from O(layers) to O(1).

## Migration

**One required env var:** set `REGISTRY_TOKEN_SECRET` (32+ random bytes)
on the registry deployment before rolling out. Boot validates it; the
service won't start without it.

**Rollout order matters.** Old astro-cli against new astro-registry:
fine. New astro-cli against *old* astro-registry: breaks (the daemon
follows the realm header, the old registry doesn't emit one). Order:

1. Set `REGISTRY_TOKEN_SECRET` on the registry.
2. Roll out the new astro-registry image.
3. Then ship the new astro-cli.

WorkOS-bearer clients (dashboard, non-Docker pulls) and old astro-cli
binaries continue to work unchanged via the legacy WorkOS path. No
client-side action required for those.
