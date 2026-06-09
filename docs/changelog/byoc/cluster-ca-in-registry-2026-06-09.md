# Store EKS cluster CA at registration for BYOC

## Summary

astro-server's per-cluster K8s client builder called `eks:DescribeCluster` on every build. That fails for BYOC clusters in customer accounts: the preview IRSA role has no cross-account DescribeCluster permission and EKS does not support resource-based policies on cluster ARNs. The K8s API path already works cross-account via explicit `aws_eks_access_entry` for astro-server — only the CA fetch needed a different source.

## Design

- **`public.clusters.eks_cluster_ca`** — PEM bytes captured once at registration (`aws eks describe-cluster` with the operator's customer-role session). `NOT NULL DEFAULT ''` so the column-add is safe; `clusterstore.validateRequiredFields` rejects empty on Register/Update.
- **`k8s.NewEKSClient`** — uses row CA when non-empty; falls back to DescribeCluster only when empty (primary cluster behavior unchanged).
- **Admin API / Queen** — Register/Update require non-empty `eks_cluster_ca` (base64 PEM in JSON). Queen Clusters UI collects it; `byoc-output.sh` fetches it when rendering the registration bundle.
- **Proto** — `bytes eks_cluster_ca` on `RegisteredCluster`, `RegisterClusterRequest`, `UpdateClusterRequest`.

Follow-up (not in this PR): scope preview IRSA `eks:DescribeCluster` to the primary cluster ARN only once the no-describe path is stable in production.

## Migration

1. Merge applies the schema column via Atlas on preview/prod.
2. For each existing additional cluster row, backfill CA via Queen Update or admin API before deploys resume (empty CA fails validation on Update).
3. New BYOC clusters: run `byoc-output.sh <customer-id>`, copy `cluster_ca` into Queen register payload as `eks_cluster_ca`.
