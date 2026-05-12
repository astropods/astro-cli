# Multi-region cluster support — data foundation

## Summary

Adds the data model for managing multiple workload Kubernetes clusters from one `astro-server`. Today the server hardcodes a single managed cluster via `EKS_CLUSTER_NAME` / `K8S_MASTER_URL`. This is the first of a series of PRs that move the server toward N clusters across regions. This PR is data-only — the table and store are added but no code reads from them yet.

## Design

### `clusters` table

Records each Kubernetes cluster `astro-server` can reconcile workloads into. Keyed by a stable string `id` (e.g. `us-east-1-managed`) that other tables and queue jobs reference. Columns:

- `id`, `region` — identity and placement.
- `eks_cluster_name`, `eks_cluster_endpoint` — what the EKS client and `aws eks get-token` need to authenticate.
- `enabled` — when false, the row is registered but cannot accept new deploys. Used to stage a cluster before promoting it.
- `created_at`, `updated_at`.

### `deployments.cluster_id`

New nullable `varchar(64)` column on `deployments` with an inline FK to `clusters(id)` `ON DELETE RESTRICT`. NULL on existing rows means "no recorded cluster" and is interpreted as the default at read time. RESTRICT ensures a cluster cannot be deregistered while deployments still reference it.

### `internal/clusterstore` package

Typed CRUD wrapper around the table: `Register`, `Get`, `List(enabledOnly)`, `SetEnabled`, `Deregister`. `ValidateID` enforces a DNS-safe id pattern. Postgres errors are mapped to typed errors (`ErrNotFound`, `ErrAlreadyExists`, `ErrInUse`) via `errors.As(&pqErr)` + SQLSTATE, matching the pattern in `handlers/knowledge.go` and `handlers/agents.go`.

## Migration

The change is expressed declaratively in `sql/astro-server/schema.sql` — Atlas diffs against the live DB and generates the required DDL on next deploy. No manual data migration is required; existing deployments keep `cluster_id = NULL` and are resolved to the default cluster at read time.
