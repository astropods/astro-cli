# Fix: bound external knowledge stores resolve the wrong host

## Summary

An agent bound to an **external** knowledge store could not reach it. The deploy-time host resolver always emitted the store's in-cluster StatefulSet service DNS, which only exists for **managed** stores. External stores have no such service, so the injected `*_HOST` pointed at nothing. PrivateLink stores were doubly broken: the only connectable address — the provisioned VPC endpoint DNS — lived on the endpoint record and was never read. External stores' credentials were also dropped, because they are stored under generic keys (`USERNAME`/`PASSWORD`/`DATABASE`) that the credential mapper, keyed on provider-specific names (`POSTGRES_USER`/…), never matched.

## Design

Host resolution for a bound store now branches on store mode instead of assuming in-cluster DNS:

- **managed** → in-cluster service DNS (unchanged)
- **external + PrivateLink** → the endpoint record's resolved `EndpointDNS` (the user-supplied `com.amazonaws.vpce.*` value is a service identifier, not a dialable host)
- **plain external** → the user-supplied `HOST` credential
- **external with neither** → explicit error rather than a silently bogus host

The decision is factored into a pure `boundKnowledgeHost(store, endpoint, creds)` helper; `resolveBoundKnowledge` now fetches the endpoint record (`GetEndpoint`) for external stores so the resolved DNS is available. The host is a deploy-time snapshot (consistent with how all bound coords are treated) — an endpoint recreate that changes the DNS requires a redeploy to pick up.

The stored `HOST` is also corrected at its source. At connect time a PrivateLink store only has the `com.amazonaws.vpce.*` service name; once the reconciler resolves the VPC endpoint, it rewrites the store's `HOST` credential to the real endpoint DNS before flipping the store to `ready`, so the persisted value is the address agents actually dial. The rewrite encrypts the new value under the store's *existing* data key (via `envelope.DecryptDataKey` + `NewEncryptorFromPlaintext`) and upserts only the `HOST` row — no re-key, other credentials untouched. Stores without an encrypted data key (KMS off) have no persisted external credentials and are skipped.

Credential mapping is unified in `mapBoundCredentials(provider, creds)`, which understands both key shapes (managed provider keys and external generic keys) and drops connection coords (`HOST`/`PORT`) so they never surface as credentials.

The reconciler's automatic rewrite only fires on the endpoint's *available* transition, so stores that became `ready` before this change keep their stale `HOST`. To repair those, a **Recheck connection** action is exposed on the store detail page (`POST /accounts/{account}/knowledge/{name}/recheck`): it re-resolves the live VPC endpoint DNS from AWS (falling back to the stored endpoint DNS) and rewrites the `HOST` credential. The credential-rewrite logic is shared by the reconciler and the handler via `knowledgestore.RewriteHostCredential`.

## Migration

None required. No spec or schema changes; existing managed-store deployments resolve exactly as before, and external/PrivateLink bindings now resolve correctly. Operators can use **Recheck connection** on any PrivateLink store connected before this change to repair its stored host on demand (deploys already resolve correctly regardless, since the deploy path reads the endpoint DNS).
