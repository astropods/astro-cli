## Summary

When an agent's pods can't be scheduled (insufficient cluster CPU/memory, taints), the deployment sits on "Deploying" and the only signal is a raw Kubernetes `FailedScheduling` event buried in the pod's Events tab — a terse code with a truncated message that doesn't tell the user the deployment is stuck or what to do about it.

## Design

The interpretation lives on the server, next to the deployment events endpoint, so it is the single source of truth and is covered by Go tests:

- A `humanizeDeploymentEvent` mapper (mirroring the existing knowledge-store `humanizeKnowledgeEvent`) translates pod event reasons into a plain-language title and guidance across the common working (e.g. `Pulling`, `Started`), transient (e.g. `Unhealthy`, `BackOff`), and error/stuck (`FailedScheduling`, `FailedMount`) states. For an identifiable cause like `FailedScheduling` the guidance states the fix plainly (reduce resources and redeploy) rather than deferring to support. Unrecognized reasons pass through untouched.
- `GetDeploymentEvents` annotates each event with the optional `title`/`guidance` fields. The raw `reason` and full K8s `message` are preserved, so the scheduling specifics ("0/14 nodes ... Insufficient memory") remain visible.
- The Events tab leads with the server title + guidance when present (and stops truncating warning messages), otherwise renders the event verbatim. No new banners or surfaces.

## Migration

No action required.
