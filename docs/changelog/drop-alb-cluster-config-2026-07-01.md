# drop ALB / ACM plumbing from cluster registration

## Summary

The tenant-router migration (see #1420, plus the front-door ALB in
astro-infra) moved TLS termination, OIDC, and DNS off the per-tenant
Ingress and onto a single shared front-door ALB. The per-Ingress
`alb.ingress.kubernetes.io/*` annotations were dropped at that point, but
the config knobs that fed them — four cluster-registration columns and
four astro-server env vars — kept being plumbed through everywhere.
This change removes that dead plumbing.

Removed everywhere:

- `agent_acm_certificate_arn`
- `agent_alb_group_name`
- `ingestion_acm_certificate_arn`
- `ingestion_alb_group_name`

And the matching primary-cluster env vars: `ACM_CERTIFICATE_ARN`,
`ALB_GROUP_NAME`, `INGESTION_ACM_CERTIFICATE_ARN`,
`INGESTION_ALB_GROUP_NAME`.

## Design

The state before was a straight-through pipe from four registry columns
through `clusterstore.Cluster` → `k8s.ClusterEntry` → `clustercfg.Resolved`
into… nothing. No consumer of `Resolved` reads `AgentACMCertARN`,
`AgentALBGroupName`, `IngestionACMCertARN`, or `IngestionALBGroupName`.
`BuildIngress` stopped emitting the ALB annotations six weeks ago; the
fields have been dormant since.

The cleanup follows the pipe end-to-end:

- **Proto (`admin.v1`).** Field tags 12/13/15/16 (RegisteredCluster),
  7/8/10/11 (RegisterClusterRequest), 6/7/9/10 (UpdateClusterRequest)
  are `reserved` so old clients don't accidentally re-use them. The
  hand-written `admin.pb.go` drops the four Go fields per struct.
- **Schema (`public.clusters`).** The four columns are removed from
  `sql/astro-server/schema.sql`; Atlas will produce a DROP COLUMN diff
  on the next apply. Because the columns had `NOT NULL DEFAULT ''`,
  existing rows just lose the (unused) empty strings.
- **astro-server.** `clusterfields.DeployConfig`, `clusterstore.Cluster`,
  `k8s.ClusterEntry`, `clustercfg.Resolved`, and
  `config.DeploymentConfig` each lose the four fields. The
  `INSERT`/`UPDATE`/`SELECT` statements in `clusterstore` shrink
  accordingly, and `admingrpc/clusters.go` stops copying the fields
  in and out of the proto messages. All handlers already ignored the
  values, so no call sites break.
- **astro-queen.** The JSON body type in `cluster_handlers.go`, the
  matching TypeScript request/response types in `web/src/types/admin.ts`,
  and the form fields in `web/src/pages/clusters.tsx` all drop the
  four fields.
- **astro-infra (Helm + Terraform).** Removes the `ACM_CERTIFICATE_ARN`,
  `ALB_GROUP_NAME`, `INGESTION_ACM_CERTIFICATE_ARN`,
  `INGESTION_ALB_GROUP_NAME` env vars from the astro-server deployment
  and worker-deployment templates, drops `managedIngress.certificateArn`
  / `managedIngress.albGroupName` / `ingestionIngress.certificateArn`
  / `ingestionIngress.albGroupName` from `values.yaml` and both per-env
  value templates, and stops threading the corresponding `managed_*` /
  `ingestion_*` template variables through the two `helm.tf` files. The
  ACM cert and ALB modules themselves stay put — the front-door ALB
  still uses them.

Tests updated in-place: the sqlmock column projections shrink to match
the new `baseSelect`, and the "missing X" table-driven cases for the
four dropped columns are removed. No new tests are needed — coverage
of the surviving fields already exists.

## Migration

Nothing operator-visible. The primary cluster stops reading the four
env vars, but they weren't feeding any behavior; safe to remove from
Helm value overrides or leave them dangling. Additional clusters
registered with values in the four columns will still deploy fine — the
columns and JSON keys are ignored on the way in, then dropped from the
table on the next Atlas apply.
