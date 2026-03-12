# Pod Restart & Stable Pod Names

## Summary

Migrates the restart-pod functionality from the deprecated operator dashboard into the new deployed agent detail page, and introduces stable pod identifiers so URLs survive pod restarts.

## Design

**Restart Pod button** is placed in the agent detail header (next to the avatar/name) and only appears when viewing a specific pod's logs. It calls the existing `POST /api/v1/deployments/:namespace/pods/:pod/restart` endpoint. On success the button swaps to a checkmark with "Restarted" for two seconds before resetting — no native browser confirmation dialog.

**Stable pod names** solve the problem of pod URLs breaking after a restart (Kubernetes assigns a new random suffix). A `getPodStableName` utility strips the trailing ReplicaSet hash and pod hash from the raw Kubernetes pod name, producing a stable identifier used in the `?pod=` query param and for route matching. A companion `getPodDisplayName` utility converts the stable name into a display-friendly string: dashes within the agent template name prefix are preserved, while dashes in the component suffix become spaces (e.g. `clawbot-ai-agent` → `clawbot-ai agent`).

## Migration

No action required. Frontend-only change using the existing restart API.
