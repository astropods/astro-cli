# Account cluster placement

## Summary

Operators assign an additional cluster to an account in Queen. Deploy and deployment-template generation inject `target.cluster_id` from that binding server-side — the CLI stays cluster-unaware.

## Design

- **Schema:** nullable `accounts.cluster_id` FK → `clusters(id) ON DELETE RESTRICT`. NULL routes deploys to the primary cluster.
- **Template injection:** `applyAccountClusterPlacement` runs on every deployment-template response (including cache hits) and in `prepareDeployment` after auth, using the **target** account (not the blueprint source account).
- **Deploy enforcement:** `prepareDeployment` sets `target.cluster_id` from the account binding before validation; existing PR 1091 checks (unknown/disabled/unhealthy) apply unchanged.
- **Admin:** `SetAccountCluster` gRPC + Queen `PUT /api/admin/accounts/{id}/cluster`; `ListAccounts` returns `cluster_id`.

## Migration

Existing accounts keep `cluster_id` NULL (primary). Assign clusters in Queen Accounts → Cluster column when ready.
