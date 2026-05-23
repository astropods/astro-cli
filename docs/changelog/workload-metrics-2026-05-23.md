# Workload metrics tab

## Summary

The pod detail panel gains a Metrics tab showing live CPU and memory
charts for the selected pod, sourced from the central Prometheus that
already aggregates cAdvisor scrapes from every Astro cluster. No new
infrastructure is required for compute metrics; storage is identified
as a follow-up.

Until now, observability for a deployed agent stopped at logs and
events. Operators had no way to spot a memory leak, see whether the
allocated CPU was being used, or compare load across the 1-hour window
before a restart — all questions Prometheus could already answer if
the data was wired through.

## Design

### Server endpoint

```
GET /api/v1/deployments/:id/pods/:pod/metrics?range=1h|6h|24h|7d
```

The handler mirrors the existing `/network/timeseries` pattern: it
re-uses `resolveDeploymentContext` for auth + namespace/cluster
resolution, then runs two parallel range queries against Prometheus —
one for CPU (`sum(rate(container_cpu_usage_seconds_total{...}[N]))`)
and one for memory (`sum(container_memory_working_set_bytes{...})`),
both scoped to `namespace="<dep.namespace>",pod="<pod>"` and filtered
to the current cluster.

```jsonc
{
  "pod": "my-agent-7d5b9c8d-rl4tk",
  "range": "1h",
  "step": "30s",
  "cpu":    [{ "timestamp": "…", "value": 0.012 }, …],   // vCPU cores
  "memory": [{ "timestamp": "…", "value": 101179392 }, …] // bytes
}
```

Range presets pick a step that keeps each query at roughly 100-250
points: 1h/30s, 6h/2m, 24h/10m, 7d/1h. `rate()` lookback is always
≥ 2× the step so a missed scrape doesn't produce empty buckets.

### Frontend

`PodDetailPanel.tsx` adds a "Metrics" tab between "Logs" and "Events".
`PodMetricsTab.tsx` renders two Area charts (CPU in cores, memory in
working-set bytes) with a small range picker on top (1h / 6h / 24h /
7d). React Query handles polling: 30s for 1h, 60s for 6h, 5min for
24h/7d.

### Storage — deferred

A direct Prometheus audit confirmed `kubelet_volume_stats_used_bytes`
and `_capacity_bytes` are present in the central Prometheus, but only
for the infra-observability cluster — they are not in the Alloy
`prometheus.remote_write` keep-list on agent clusters. Adding storage
charts is therefore an astro-infra change (one-line addition to the
keep-list) plus a UI follow-up, not part of this PR. The tab footer
notes that storage is coming.

## Migration

None required.

- `PROMETHEUS_URL` and `EKS_CLUSTER_NAME` must be set on the deploy
  environment for charts to populate; both are already wired through
  `config.Deployment`. With either missing, the endpoint returns
  `{cpu: [], memory: []}` rather than 5xx, so the UI degrades to "No
  data in this range." rather than crashing.
