# Queen placement visibility (API + UI)

## Summary

Operators could not see when account placement (`accounts.cluster_id`) and deployment routing (`deployments.cluster_id`) diverged — drift could look healthy on primary while the account was pinned to EU. Admin gRPC now exposes placement fields; Queen list and detail surfaces them with a mismatch banner and live-cluster label.

## Design

**Server**

- **`AdminDeployment`**: `cluster_id`, `account_cluster_id`, `placement_mismatch` (normalized: `""` and `"primary"` are equivalent).
- **`GetDeploymentResponse.placement_hint`**, **`GetClusterStatusResponse.resolved_cluster_id`** (K8s API target for the namespace).
- **`ListAllWithAccount`**: joins deployment and account cluster columns.

**Queen**

- Deployment detail: deployment/account cluster info cards highlight mismatch (amber border, cross-reference hints), subtitle for `resolved_cluster_id`, optional ECR region vs deployment cluster line from the agent image.
- Deployments list: cluster column, mismatch badge with tooltip, cluster ids in search.

## Migration

None. No deploy or Redeploy behavior change.
