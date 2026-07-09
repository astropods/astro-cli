# Registry resolves the pull namespace (name or id)

## Summary

Moves tenant name→repo mapping entirely into astro-registry, where account↔repo knowledge already lives. Previously the CPC pull path assumed the image namespace was already an account **id**, which forced astro-server to rewrite pushed references (parse the account out, swap in a frozen `ECRNamespace`) before deploy. That parsing is being removed from the server (see server pull-through change); this makes the registry resolve the namespace itself, so whatever reference the spec carries "just works".

## Design

- The CPC `/token` path replaces the id-only `HomedHere` check with `ResolveHomedAccount(namespace, clusterID)`: it looks the account up by **name** (the developer-facing namespace in a pushed reference) or by **id** (references rendered before the server stopped rewriting), then applies the same homing rule (`accounts.cluster_id`, `NULL` = primary).
- The resolved account **id** is placed in the minted token's access entry, so the proxy's existing `{env}-tenant-{id}` rewrite lands on the correct ECR repo regardless of whether the pushed namespace was a name or an id.
- This mirrors what the push / WorkOS pull path already does (name→id), so both directions share one mapping home.

## Migration

None. Backward compatible: id-namespaced references (current deployments) still resolve via the id branch; name-namespaced references (after the server change) resolve via the name branch. Account rename/transfer history, if ever needed, is a registry-side concern (alias table) — not the control plane's.
