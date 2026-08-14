## Summary

Every unbound account (`accounts.cluster_id IS NULL`) was failing CPC pull authorization on any environment running cluster-config boot sync — a live, active bug, not an edge case.

astro-server's boot sync removed the primary-vs-additional split: the default cluster is just another `clusters` row, and `clustercfg.Resolve` returns that row's real, DB-backed pull credential for default-routed deployments. So the CPC a default-routed pod presents is `astrocp_{real-default-cluster-id}_{secret}` (for example `astrocp_preview-managed-eks_{secret}`), never the literal `astrocp_primary_{secret}` sentinel.

astro-registry's `clusterpull.Authorizer` never learned about that change. `Authenticate` already worked, since it looks up whatever id is actually presented against `clusters.pull_key_hash`. `ResolveHomedAccount` didn't: it only treated the literal string `"primary"` as "this is the primary cluster, home unbound accounts here" — a value that cluster-config boot sync never actually presents. Every other cluster id, including the real default cluster's, fell through to the equality check against `accounts.cluster_id`, which is `NULL` for an unbound account and so never matches anything. Every unbound account's pulls authenticated fine and then got authorized with zero scopes.

## Design

`Authorizer` now takes a `defaultClusterID` (astro-server's `DEFAULT_CLUSTER_ID`, sourced from the identical env var on astro-registry) alongside the existing `primaryHashHex`. A new `isPrimary` helper treats the literal `"primary"` sentinel and the configured default cluster's real id as equivalent for homing purposes:

```go
func (a *Authorizer) isPrimary(clusterID string) bool {
	return clusterID == PrimaryClusterID || (a.defaultClusterID != "" && clusterID == a.defaultClusterID)
}
```

`ResolveHomedAccount` uses `isPrimary` instead of the raw sentinel comparison. `Authenticate` is unchanged — its DB-row lookup for a real cluster id already worked correctly and isn't part of this bug.

## Migration

astro-registry's deployment needs a `DEFAULT_CLUSTER_ID` env var set to the same value as astro-server's, on every environment running cluster-config boot sync (preview, prod). Without it, `defaultClusterID` stays empty and behavior is unchanged from before this fix — safe to deploy the code first, but the fix only takes effect once the env var is wired in astro-infra's Helm values for astro-registry (a follow-up in that repo; not part of this change).
