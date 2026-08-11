# Fix tenant NetworkPolicy egress/ingress on IPv6 clusters

## Summary

`pm-eu` (`preview-managed-eu-west-1-a`), astro's first IPv6 pilot cluster, had every tenant pod silently unable to resolve DNS or reach the internet. The per-namespace `allow-namespace-traffic` NetworkPolicy astro-server generates for every tenant deployment allows "everything else" (DNS to kube-dns, outbound internet) via a single `ipBlock` hardcoded to `0.0.0.0/0`. Kubernetes `ipBlock` CIDRs only match traffic in their own address family, so on an IPv6 pod network that rule matched nothing — combined with the namespace's `default-deny-all` policy, this blocked all IPv6 egress, including to `kube-dns` (which lives in a different namespace, so it isn't covered by the policy's same-namespace pod-to-pod rule either).

## Design

`pod_subnet_cidrs` (the except-list that keeps the broad IPv4 allow-rule from also opening access to other tenants' pod IPs) gets a new IPv6 sibling, `pod_subnet_ipv6_cidrs`, threaded through the same path end-to-end:

- **astro-infra**: `managed-cluster-infra` module exposes a `pod_subnet_ipv6_cidrs` output (private + secondary subnet IPv6 CIDRs, empty string for IPv4-only clusters), plumbed through `cluster-registration-payload` into every managed cluster's `registration.json`.
- **Proto/admin API**: `RegisteredCluster`, `RegisterClusterRequest`, `UpdateClusterRequest` gain the field (astro-proto's admin types are hand-maintained JSON-over-gRPC structs, not codegen'd).
- **astro-server**: field flows through `clusterstore` (new `pod_subnet_ipv6_cidrs` column, `NOT NULL DEFAULT ''`) → `clustercfg.Resolve` → `k8s.ApplierConfig` → `Applier`.
- **The actual fix**, in `Applier.applyNetworkPolicies`: when `podSubnetIPv6CIDRs` is non-empty, a second `ipBlock` peer (`CIDR: "::/0"`, `Except: podSubnetIPv6CIDRs`) is added alongside the IPv4 one, in both the ingress `From` and egress `To` of the "everything else" rule. IPv4-only clusters (every cluster except `pm-eu` today) get a byte-identical policy to before — the new peer only appears when the field is populated.
- **astro-queen**: the cluster register/edit form and API proxy gained the field (optional — every field in `clusterDeployBody` except this one is still required, since it's only meaningful for IPv6 clusters).

The field is optional everywhere (default `""`), so it's a no-op for existing IPv4-only clusters and doesn't touch the required-field validation astro-server already enforces for `pod_subnet_cidrs`.

## Migration

No action required for existing clusters — the new column defaults to empty and the NetworkPolicy generator only adds the IPv6 peer when a value is present. `pm-eu`'s `registration.json` has already been regenerated with its real IPv6 pod-subnet CIDRs (astro-infra `preview-managed-eu-west-1-a-infra` was applied directly); its cluster row needs an `UpdateCluster` call (or re-registration) once this PR ships to pick up the fix.
