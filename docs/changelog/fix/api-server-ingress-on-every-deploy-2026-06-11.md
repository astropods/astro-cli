# Apiserver proxy NetworkPolicy on tenant namespaces for in-client chat

## Summary

In-client deployment chat reaches messaging sidecars through the Kubernetes API service proxy (`/api/v1/namespaces/{ns}/services/{svc}:8090/proxy/...`). RBAC for `services/proxy` was necessary but not sufficient: tenant `allow-namespace-traffic` NetworkPolicies excluded EKS apiserver ENI subnets from the external ingress allow rule, so proxy connections to messaging pods were dropped and astro-server saw hangs → CloudFront 504s.

## Design

**Root cause:** On managed clusters, pods run in secondary-private CIDRs while apiserver ENIs sit in primary VPC private subnets. When those primary subnets were included in `POD_SUBNET_CIDRS` (the netpol `except` list), apiserver-origin traffic was explicitly denied despite the `0.0.0.0/0` ingress rule.

**Fix:** Add `CP_SUBNET_CIDRS` (primary VPC private subnets hosting apiserver ENIs) to deploy-time config. On every namespace apply, astro-server now emits a sibling NetworkPolicy `allow-apiserver-proxy` alongside the existing `allow-namespace-traffic`. The sibling NP is intentionally narrow: `podSelector: app.kubernetes.io/component=agent` restricts the destination to messaging sidecar pods, source ipBlocks restrict to apiserver ENI CIDRs, and ports restrict to TCP **8090** (messaging HTTP — the service-proxy path in-client chat uses). gRPC 9090 is intentionally not exposed via apiserver proxy. The NP is omitted when the env var is unset (local dev, clusters without netpol isolation).

Why a sibling NP rather than mutating `allow-namespace-traffic`: keeping the apiserver allow in a separate, scoped policy avoids exposing any future pod in the namespace bound to 8090/9090. Defense in depth — even if a non-messaging workload happens to listen on those ports, it isn't selected by the sibling policy. `allow-namespace-traffic` stays unchanged in shape.

Wiring: `DeploymentConfig` → `clustercfg.Resolve` (primary cluster only today) → `Applier` → `apiserverProxyNetworkPolicy` helper in `applyNetworkPolicies`. Existing namespaces pick up the rule on the next deploy/redeploy that touches the namespace.

**Companion infra (astro-infra, separate PR):** Split `pod_subnet_cidrs` to secondary-private only, narrow the managed EKS cluster's `subnet_ids` to primary subnets so apiserver ENIs are pinned there, and pass `cp_subnet_cidrs` into astro-server helm as `CP_SUBNET_CIDRS`.

## Migration

1. Deploy astro-infra change so preview/prod astro-server receives `CP_SUBNET_CIDRS` (primary private subnet CIDRs for the managed cluster) and `POD_SUBNET_CIDRS` is narrowed to secondary-private only.
2. Deploy astro-server from this branch.
3. Redeploy agents (or wait for natural redeploys) so tenant namespaces get the new `allow-apiserver-proxy` policy and the updated `allow-namespace-traffic` except list.

No database or client changes.
