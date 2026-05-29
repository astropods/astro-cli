# Queen multi-cluster visibility 

## Summary

Completes the Queen multi-cluster admin plan after placement API + core UI: ops polish (jobs cluster column, cluster deployment counts, account navigation, spec callout), deployments list cluster/mismatch filters with shareable URLs, Redeploy sync of `deployments.cluster_id` from account placement, and cluster ingress completeness badges.

## Design

- **River jobs:** `GetDeploymentJobs` returns `args.cluster_id` so operators can see which cluster a deploy/undeploy/wakeup job targeted.
- **Redeploy:** When `accounts.cluster_id` ≠ `deployments.cluster_id` (normalized), admin re-apply patches stored spec `target.cluster_id`, updates the deployment row, records a timeline event, then enqueues the deploy job on the account cluster. Response includes `cluster_placement_updated` and an operator message; Queen warns that pods may remain on the old cluster until the worker runs.
- **List filters :** `?cluster=` and `?mismatch=1` on the deployments page; cluster dropdown uses registered clusters plus primary.
- **Clusters:** Active deployment counts per routed cluster (client-side from list); non-primary rows show **Ingress incomplete** when any required ingress field is empty, linking to the edit dialog.
- **`account_id` on `AdminDeployment`:** Enables account deep-links from deployment detail (`/admin/accounts?q=`).

## Migration

None. Deploy astro-server + astro-queen together so Redeploy placement sync and new proto fields are available.
