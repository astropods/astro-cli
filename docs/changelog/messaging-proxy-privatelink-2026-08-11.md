# Route the in-app chat proxy over PrivateLink instead of the K8s apiserver

## Summary

astro-server's chat/messaging/files proxy handlers (`ProxyDeploymentMessaging`, `ForwardChat`, and the files proxy) reached a tenant's messaging sidecar by issuing a Kubernetes `services/{name}:{port}/proxy` request through the cluster's API server. On an IPv6-only managed cluster this proxy command hangs outright, and on every managed cluster the tenant's own `allow-namespace-traffic` NetworkPolicy can block the API server's relay connection because it originates from the cluster's own subnets. astro-infra built a private AWS PrivateLink path straight to the tenant-router Envoy that avoids both problems (see `docs/plans/internal-tenant-router-nlb.md` in the astro-infra repo); this change makes astro-server use it.

## Design

Each cluster can now store a `tenant_router_internal_url` (address:port of its private Envoy endpoint), mirroring the existing per-cluster `loki_url`/`prometheus_url` override pattern end to end: `clusters` table column → `clusterstore.Cluster` → `k8s.ClusterEntry` → admin proto/API → astro-infra's `registration.tf`.

`resolveMessagingProxyTarget` now checks for this address first. When set, it builds a plain HTTP request straight to that address, with the `Host` header set to a new internal-only host (`GenerateMessagingInternalHost`, `<namespace>.messaging.internal`) instead of the tenant's public hashed chat hostname. The tenant-router Envoy routes purely on `Host`, and this value never needs to be secret or resolvable — the internal load balancer's security group is the actual boundary, not the hostname. The messaging `Ingress` object gets this second host as an extra rule (`IngressConfig.ExtraHosts`) pointing at the exact same backend as the public one.

When no per-cluster address is stored, the proxy falls back to the original API-server `services/proxy` path unchanged. The primary cluster has no `clusters` row of its own, so it reads a new `TENANT_ROUTER_INTERNAL_URL` env var instead; today that var is unset, so the primary cluster keeps using the old path until astro-infra builds this network path for it too.

## Migration

Atlas will apply `ALTER TABLE clusters ADD COLUMN tenant_router_internal_url varchar(512) NOT NULL DEFAULT ''`. Existing rows get an empty string, which keeps every cluster on the old API-server proxy path until astro-infra's `registration.tf` supplies a real address (already done for both preview managed clusters). No action needed for the primary cluster or any cluster without the new network path built.
