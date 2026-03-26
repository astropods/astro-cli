# Move deployment pages from SSR to client-side data fetching

## Summary

The deployment detail and configure pages blocked SSR on `listDeployments`, which fans out to multiple sequential K8s API calls per deployment (namespace lookup, deployments, statefulsets, ingresses, pods). On Azure clusters where K8s responses are slow, this caused multi-second blank pages before any HTML was sent.

## Design

SSR loaders for `DeployedAgentDetail` and `DeployedAgentSettings` now return only route params (`account`, `deploymentId`) — no API calls. Deployment data is fetched entirely client-side via the existing `useDeployments` TanStack Query hook, which was already running after hydration anyway.

A full-page skeleton mirrors the monitor tab layout (top bar, tab bar, headline metric cards, request volume / token usage panels, traces grid) so the page appears structurally complete while data loads. A shared `MetricCardSkeleton` component is used by both the page-level skeleton and the `HeadlineMetrics` internal loading state, eliminating layout shift when the real cards render.

## Migration

No changes required. Pages load the same data — just client-side instead of server-side.
