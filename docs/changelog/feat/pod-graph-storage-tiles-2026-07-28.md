# Knowledge storage on pod-graph tiles

## Summary

Knowledge container tiles in a deployment's pod graph now show live persistent-volume usage — a `used / capacity` readout with a fill bar — so operators can see at a glance how full a knowledge store's disk is without opening the pod detail panel's metrics tab.

## Design

**Data source.** Reuses the `kubelet_volume_stats_used_bytes` / `kubelet_volume_stats_capacity_bytes` series already scraped into Prometheus (the same metrics the per-pod storage chart uses). No new metric, scrape, exporter, or Kubernetes API read is introduced.

**Server — enrich the runtime endpoint, keep it cheap.** `GET /deployments/:id/runtime` was a pure DB-snapshot read. It now overlays per-workload storage via a **single namespace-scoped instant Prometheus query per deployment**, scoped to exactly that deployment's StatefulSet PVCs. Because a StatefulSet's volume-claim PVC is deterministically named `data-<pod>`, each returned series maps back to its workload with no K8s lookup. Results are memoized in a process-wide TTL cache (15s) fronted by singleflight, so many viewers and fast polls of the same pod graph collapse to roughly one query per deployment per TTL. The fetch is time-boxed and degrades to *absent* — never an error, never a 503 — when Prometheus is unavailable or a workload owns no volume, preserving the endpoint's "renders instantly, cluster-independent" contract. Each runtime workload gains optional `storage_used_bytes` / `storage_capacity_bytes`.

**Client — render only where it's meaningful.** The new fields flow through the existing spec-plus-runtime merge onto `WorkloadDetail`. Knowledge-role tiles render a compact fill bar whose color escalates neutral → amber (≥80%) → red (≥95%) as the volume fills, echoing the "writes fail when full" warning. The bar is hidden while the deployment is paused/probing and when metrics are absent, so non-persistent knowledge (e.g. cloud providers) and other roles show nothing.

## Migration

None. No configuration, API contract, or schema changes. The readout appears automatically in any cluster where kubelet volume stats are already scraped into Prometheus.
