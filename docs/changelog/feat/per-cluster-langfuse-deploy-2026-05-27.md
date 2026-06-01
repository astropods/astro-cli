# Per-cluster Langfuse URL and netpol config for deploys

## Summary

Deploys to additional clusters used preview-primary `LANGFUSE_BASE_URL_EXT`, `LANGFUSE_VPCE_IPS`, and `POD_SUBNET_CIDRS` for every cluster. Collectors in other VPCs could not reach Langfuse, so agent card sparklines stayed flat even when workload metrics worked. Resolution is per `clusters.id` (same pattern as per-cluster Loki/Prometheus), not tied to a single region.

## Design

- **`public.clusters`** gains `langfuse_base_url_ext`, `langfuse_vpce_ips`, `pod_subnet_cidrs` (required for non-primary rows, same as ingress fields).
- **`clustercfg.Resolve`** returns Langfuse base URL, VPCE IP list, and pod subnet CIDRs; primary still reads env vars.
- **`deployer.Apply`** passes resolved values into `ApplierConfig` for collector `LANGFUSE_BASE_URL` and tenant NetworkPolicy egress.
- **Admin API / Queen**: Register/Update cluster forms collect the three fields.

Pair with astro-infra Langfuse PrivateLink on each target managed cluster; populate values from that cluster's `*-infra` Terraform outputs after apply.

## Migration

Roll out in one coordinated window per environment: schema, cluster row backfill, and astro-server deploy should land before (or together with) traffic that re-applies additional-cluster deployments. Between schema apply and backfill, `clustercfg.Resolve` hard-fails deploys on additional clusters with empty Langfuse fields (intentional). Preview today is a single additional cluster row; backfill that row (Queen, admin API, or SQL) before or in the same release as the server binary.

1. Apply DB schema (`sql/astro-server/schema.sql` via Atlas).
2. For each additional cluster: backfill Langfuse URL, bare VPCE IPs (comma-separated, no `/32`), and pod subnet CIDRs from `*-infra` / `byoc-output.sh` outputs.
3. Deploy astro-server build that includes this change.
4. Re-apply or redeploy agents on those clusters so collector sidecars pick up the new env and netpol.
