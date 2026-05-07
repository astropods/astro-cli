# fix-ingestion (2026-05-07)

## Summary

Follow-up polish on the previous day's CronJob/Job-as-Workload change.
Three rough edges, all driven by the new workload kinds reaching the
deployment view's pod tile and detail panel:

1. The pod tile and detail panel crashed on ingestion workloads because
   `WorkloadDetail.Containers` came back as JSON `null` — Go's nil slice
   serializing through a non-nullable TS type — and the client called
   `.reduce` / `.map` on it directly.
2. Even past the crash, ingestion tiles showed "Starting" because pod
   status was derived from container readiness, but Job/CronJob health
   doesn't live there — it's in the workload's own `Status` field.
3. The General tab's Environment Variables section was empty for
   Job/CronJob workloads even when their templates clearly declared
   env. Server populated `Containers` only for Deployments/StatefulSets
   from the running pod's spec; Jobs/CronJobs were skipped entirely.

## Design

**Client: kind-aware status derivation.** `derivePodStatus` now returns
`{ status, label }` and switches on `WorkloadDetail.kind`:

- `Job` (`Pending` / `Running` / `Succeeded` / `Failed`) maps to
  `pending` / `healthy` / `healthy` / `unhealthy` with labels
  `Pending` / `Running` / `Completed` / `Failed`.
- `CronJob` (`Idle` / `Active` / `Suspended`) maps to `healthy` /
  `healthy` / `warning` with labels `Idle` / `Running` / `Suspended`.
- `Deployment` / `StatefulSet` keep the original container-readiness
  derivation and `Online` / `Degraded` / `Error` / `Starting` labels.

`PodTileContent` accepts an optional `statusLabel` to override the
default per-status label so the same colored dot can render different
text based on workload kind. The `Download` lucide icon represents both
`Job` and `CronJob` kinds (rather than special-casing the
`ingestion-<name>` component prefix).

The two crash sites are guarded with `?? []` / inlined non-null checks.
The TypeScript type still says `containers: ContainerStatus[]`; the
guards exist because Go's JSON encoding doesn't honor that contract.

**Server: env-var resolution decoupled from the live pod.** Env-var
extraction was previously embedded in `buildContainerStatuses`, which
took a `corev1.Pod` — meaning env vars only appeared when a pod existed.
For CronJobs that haven't fired and Jobs whose pods have been GC'd that
gave an empty Environment Variables panel.

The fix splits the env logic out:

```go
// PodSpec is the only thing env resolution actually needs.
func resolvePodSpecEnv(ctx, clientset, ns, podSpec) map[string][]EnvVar
```

`buildContainerStatuses` now delegates to this helper (no behavior
change for live pods). A new `containersFromSpecWithEnv` seeds
`Containers` from a workload's own pod template with names + resolved
env. The CronJob and Job listing loops use this helper so their
`Containers` are always populated.

The pod-match enrichment loop is unchanged: when a live pod is matched,
its runtime entries (which include env resolved from the same PodSpec)
overlay the seeded list. We accept the architectural split — env from
spec for batch kinds, env from running pod for long-running kinds —
because the running pod and the spec are typically identical and the
batch-kind path needs to work without a pod at all.

## Migration

None. All changes are internal to the deployment view path. API shape
of `WorkloadDetail` is unchanged from the previous day's commit.
