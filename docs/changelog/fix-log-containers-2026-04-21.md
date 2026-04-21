## Summary

Two related fixes for workload container data missing from deployment details, particularly for StatefulSets using the `OnDelete` update strategy.

## Design

### Always populate container names from the K8s Deployment/StatefulSet manifest

When listing workload details, container names are now seeded from the K8s Deployment or StatefulSet manifest spec (`spec.template.spec.containers`) returned by the K8s API list call. This is the live manifest applied to the cluster — not the Astro spec (`astropods.yml`) or the `deployment_workloads` DB table. The manifest is already fetched as part of the existing K8s list call, so there is no extra round trip.

This guarantees that every workload always has at least the container names present in the response. If a matching running pod is found, `enrichContainerStatuses` merges the full runtime fields (state, ready, restart count, reason, env) into the manifest-seeded list.

### Match pods by agent and component, not version

Pod matching previously keyed on `agent + version + component`. For `OnDelete` StatefulSets, the running pod retains its old version label after a redeploy, so it was never matched and no runtime status was returned.

Version is now excluded from the pod index key. When multiple pods exist for the same agent and component, the most current one is selected — preferring Running phase, then newest by creation timestamp.

## Migration

No action required.
