# Registry pull-through — cluster pull credential path

## Summary

First half of registry pull-through (see `docs/01-spec/registry-pull-through-spec.md`), split out so it can deploy and bake independently of the server-side behavior change. This adds the mechanism to `astro-registry` and is **backward-compatible and inert**: existing WorkOS push/pull is unchanged, and the new path only activates when a cluster presents a pull credential.

## Design

- **Cluster pull credential (CPC).** `astro-registry`'s `/token` endpoint gains a second issuance path, selected when the Basic password is a `astrocp_{clusterID}_{secret}` value (vs a WorkOS token). The CPC is a per-cluster, pull-only machine credential.
- **Authentication.** The registry hashes the secret and compares against `PRIMARY_PULL_KEY_HASH` (the reserved `primary` clusterID) or `clusters.pull_key_hash` (additional clusters). Overwriting the stored hash revokes one cluster in isolation.
- **Cross-cluster isolation.** For each requested repo, the registry resolves the account and grants `pull` only when that tenant is homed on the requesting cluster (`accounts.cluster_id`; `NULL` = primary). A compromised cluster can pull only its own tenants, never another's.

## Migration

Additive: new optional `PRIMARY_PULL_KEY_HASH` env and a new nullable `clusters.pull_key_hash` column. No behavior change until the server begins resolving images to the proxy host (separate change) and clusters carry a CPC. Safe to deploy and leave inert.
