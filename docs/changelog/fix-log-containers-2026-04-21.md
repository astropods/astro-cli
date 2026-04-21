## Summary

Containers were missing from deployment details when a StatefulSet used the `OnDelete` update strategy and its pod had not been recreated after a redeploy. Because the pod's version label didn't match the current build, the workload was listed with no containers at all.

## Design

Container list is now seeded from the pod template spec at workload-listing time (`containersFromSpec`), guaranteeing at least name-level presence for every container. If a running pod is found and version-matched, `enrichContainerStatuses` merges the runtime fields (state, ready, restart count, reason, env) into the spec-seeded list. Containers present in the spec but absent from runtime are returned with zero runtime fields rather than omitted.

The spec is treated as the authoritative source for _which_ containers exist; runtime status is treated as optional enrichment.

## Migration

No action required.
