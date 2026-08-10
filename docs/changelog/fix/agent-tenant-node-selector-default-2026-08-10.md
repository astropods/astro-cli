# Default agent workloads to the tenant node pool

## Summary

Cluster nodes are pooled by `workload-type` label (system, tenant, observability, build, gpu). Agent pods got a toleration for the system pool's taint but no node selector, so with no explicit config or GPU requirement they could schedule onto any untainted pool — including system, which is reserved for cluster fabric (CoreDNS, the ALB controller, Contour). This filled system pools and starved fabric components.

## Design

`BuildDeployment` and `BuildStatefulSet` already chose a `NodeSelector` in priority order: explicit config, then GPU auto-detection (`workload-type: gpu`), then nothing. Added a third fallback — `workload-type: tenant` — when neither of the first two applies. Explicit config and GPU precedence are unchanged; only the empty-selector case changes.

Grepping for the same "explicit-or-GPU-or-nothing" pattern surfaced four more pod-spec builders with no selector at all: `BuildJob` and `BuildIngestionDeployment` (ingestion), `BuildCronJob` (scheduled ingestion), and `BuildCollectorDeployment` (per-agent OTel sidecar). These have no explicit-selector or GPU concept, so they now set `workload-type: tenant` unconditionally.

`githubbuild/builder.go`'s build pods were left untouched — they already set `workload-type: build` explicitly.

## Migration

None. Takes effect on next deploy/apply of each resource; no action required.
