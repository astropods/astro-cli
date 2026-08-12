# Per-cluster Loki/Prometheus query endpoints

## Summary

astro-server queried Loki and Prometheus/VictoriaMetrics through exactly one global endpoint pair (`LOKI_URL`/`PROMETHEUS_URL`), regardless of which cluster a deployment actually lived on. That was fine while every cluster shipped into one shared observability stack, but managed clusters now get their own local Loki + VictoriaMetrics behind a dedicated PrivateLink query endpoint (astro-infra), and an environment can have more than one managed cluster. With a single global URL, only one cluster's store could ever be reachable at a time — logs/metrics for deployments on any other cluster would silently come back empty, and the message-count billing worker only ever metered the primary cluster's series.

## Design

`loki_url` and `prometheus_url` join the existing per-cluster fields (`agent_ingress_domain`, `langfuse_base_url_ext`, `pod_subnet_ipv6_cidrs`, ...) on the `clusters` table, following the same optional-field pattern as `pod_subnet_ipv6_cidrs`: empty means "no override, use the global env var," non-empty means "this cluster has its own observability stack."

Flow: `clusters` table → `clusterstore.Cluster` → `k8s.ClusterEntry` (via the shared `ClusterEntryFromRow`, one conversion site) → new `k8s.Registry.LokiClientFor` / `PrometheusClientFor`. Both take a `deploymentClusterID` and a default client; they resolve the cluster's entry and return a cached client built from its `loki_url`/`prometheus_url` if set, otherwise the default. This mirrors the existing `PrometheusClusterFilter`/`LokiClusterName` helpers, and is nil-safe the same way.

Call sites that resolve a deployment's cluster now resolve its observability client the same way: deployment log streaming and tailing (`GetDeploymentLogs`, `StreamDeploymentLogs`), runtime storage metrics (`GetDeploymentRuntime`), network summary/flows/timeseries (`resolveDeploymentContext`, now carrying a `PromClient` field), workload metrics, and the admin `GetPodLogs` RPC. Each still falls back to the global client when the deployment's cluster has no override, so this is additive — no behavior change for clusters without `loki_url`/`prometheus_url` set.

The message-count sync worker (billing) had the sharpest version of this bug: it queried only the primary cluster's Prometheus, via a `cluster` label baked into the client at construction. It now lists every enabled cluster via `Registry.List` and queries each one's own client (falling back to the default when unset), summing results across clusters before upserting message counts — so agents on additional clusters are actually metered.

`ListOutboundDomains` (admin RPC backing brand-icon prioritization) had a subtler version of the same bug: it already built a PromQL label selector spanning every registered cluster's name, but ran it as one query against the single default client — a label filter can't reach series that live in a *different Prometheus entirely*, so any cluster with its own `prometheus_url` was silently excluded no matter how the selector was built. It now groups registered clusters by which client actually answers for them (the shared default, or a distinct client per override) and queries each backend separately, merging results before aggregation.

Both of these callers already have the `k8s.ClusterEntry` in hand from `Registry.List`, so they resolve clients via a new `LokiClientForEntry`/`PrometheusClientForEntry` pair rather than the id-based `LokiClientFor`/`PrometheusClientFor` — the id-based versions call `GetEntry`, which (unlike `List`) doesn't share a cache, so calling them per entry in a loop would cost one redundant `clusterstore` round trip per cluster on every run.

Fleet-wide, identity-less aggregation with no per-account/per-deployment scoping at all (alert-condition PromQL evaluation) is left untouched — there's no cluster to resolve there in the first place. `insights.go`'s devtool-cost tracking (`claude_code.cost.usage` etc.) was checked too: those metrics are ingested centrally from local CLI usage via astro-otel, not sourced from any K8s cluster, so per-cluster fan-out doesn't apply to them.

The admin API (`RegisterCluster`/`UpdateCluster`/`RegisteredCluster` proto messages, hand-maintained JSON-over-gRPC, not codegen'd) and astro-queen's cluster register/edit form both gained the two fields, same as every other optional per-cluster field.

## Migration

No action required for existing clusters — the new columns default to `''`, and both new client-resolution helpers fall back to the global `LOKI_URL`/`PROMETHEUS_URL` when empty. To point a specific cluster at its own observability stack, set `loki_url`/`prometheus_url` via `UpdateCluster` (or the queen UI) once astro-infra has provisioned that cluster's query endpoint.
