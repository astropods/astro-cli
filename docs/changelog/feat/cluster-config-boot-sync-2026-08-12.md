## Summary

Cluster registration was manual: astro-infra rendered a JSON payload per cluster, and an operator pasted it into astro-queen to call an admin RPC. This also meant the cluster astro-server itself runs on ("primary") was structurally different from every other cluster — env-var-configured, no row, special-cased throughout the registry and admin API.

## Design

astro-infra now aggregates every cluster's registration payload (including the one astro-server runs on) into a single `cluster-config.json`, mounted into astro-server's pods via a `ConfigMap`. At boot, astro-server reads it once and upserts each cluster's connectivity fields into `public.clusters` — before serving any traffic. `DEFAULT_CLUSTER_ID` names which entry is the cluster astro-server itself is running on; `k8s.Registry` builds that entry's `ClusterClient` directly instead of relying on dedicated `EKS_CLUSTER_NAME`/`K8S_MASTER_URL` env vars.

There is no more primary-vs-additional split: every cluster is a row, keyed by real id, with pull-credential fields staying server-owned (never touched by the sync). There's also no enabled/disabled state — every row present is usable. If a cluster's config disappears, boot sync deletes its row on the next boot, same FK-gated semantics as a manual `DeregisterCluster` call: blocked (and logged) if an account or deployment still references it, deleted otherwise. `RegisterCluster`/`UpdateCluster`/`EnableCluster`/`DisableCluster` are removed entirely — boot sync is the only writer of connectivity and availability state now, closing the unreviewed-write-path problem those RPCs left open. astro-queen's cluster page drops the register/edit/enable/disable actions to match; it's read-only aside from deregister and health-check.

One field stays conditionally required: `eks_cluster_ca` is only enforced for non-default clusters, since the default cluster is the one astro-server runs inside and never needs a stored cross-account CA. `langfuse_vpce_ips` is optional for every cluster — only ones needing a PrivateLink netpol exception to reach Langfuse set it. Every other field (ingress domains, Langfuse URL, pod subnet CIDRs) is required for every cluster, default or not.

`k8s.Registry` no longer synthesizes a placeholder entry for the default cluster when its `clusters` row is missing. Earlier drafts of boot sync fell back to a config-only entry so the registry stayed usable while a row was pending; in practice that fallback masked a real sync failure (the CA/VPCE validation above) behind a fake "healthy" cluster with no way to tell it apart from a real one. A missing default-cluster row now behaves exactly like any other cluster's missing row: lookups fail, and it's absent from list results. `outboundClusterGroups` and the message-count sync worker, the two callers that unintentionally depended on the fallback for the default cluster's own Prometheus coverage, now source that coverage from the process's own Prometheus client directly instead.

Several env vars that duplicated data already available per-cluster are gone: `EKS_CLUSTER_NAME`, `K8S_MASTER_URL`, `INGRESS_DOMAIN`, `INGESTION_INGRESS_DOMAIN`, `AGENT_INGRESS_PUBLIC_DOMAIN`, `POD_SUBNET_CIDRS`, `POD_SUBNET_IPV6_CIDRS`, `LANGFUSE_BASE_URL_EXT`, `LANGFUSE_VPCE_IPS`, `TENANT_ROUTER_INTERNAL_URL`, `LOKI_URL`, `PROMETHEUS_URL`, `DEPLOYMENT_LOG_BACKEND`. Each had the same shape: an env var read only when a deployment's `cluster_id` was NULL, standing in for data the cluster's own row already carries once every cluster (including the default) is synced uniformly.

Full design: `docs/01-spec/cluster-registration-config-spec.md`.

## Migration

No action for existing deployments — routing behavior for `cluster_id IS NULL` is unchanged. Onboarding a new cluster (or changing an existing one's connectivity) now requires only landing the astro-infra Terraform change and a normal astro-server redeploy; the old astro-queen paste-into-console step is gone.
