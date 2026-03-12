# Colocate messaging and collector as sidecar containers in the agent pod

## Summary

Messaging and collector were deployed as separate Kubernetes Deployments (each with their own pod), which was wasteful in terms of scheduling overhead, network latency, and resource consumption. This change colocates them as sidecar containers within the agent pod.

## Design

Instead of creating 3 independent Deployments per agent (agent, messaging, collector), the server now builds a single Deployment with up to 3 containers in the same pod:

- **`app`** — the main agent container (always present)
- **`messaging`** — messaging/interfaces sidecar (when `interfaces.adapters` is configured)
- **`collector`** — observability collector sidecar (when `observability.enabled` is true)

`DeploymentConfig` gains two optional fields (`Messaging`, `Collector`) that, when non-nil, cause `BuildDeployment` to append the corresponding container to the pod spec. The removed `BuildMessagingDeployment` and `BuildCollectorDeployment` functions are no longer needed — their container builder functions (`buildMessagingContainer`, `buildCollectorContainer`) are reused directly.

Services for messaging and collector still exist for ingress routing and discoverability, but their selectors now target `component: agent` (the agent pod) instead of separate `component: messaging`/`component: collector` pods. Since all containers share the same network namespace within a pod, the agent can reach messaging and collector via `localhost` instead of cross-pod DNS.

## Migration

No user-facing changes. Existing deployments will be reconciled on next deploy — the stale standalone messaging/collector Deployments are cleaned up by the existing `cleanupStaleBuildResources` mechanism.
