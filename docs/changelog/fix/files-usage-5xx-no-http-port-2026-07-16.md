# Files API: don't 5xx when a deployment has no file storage

## Summary

The `/api/v1/deployments/:id/files/*` routes returned **503** for any deployment
whose messaging Service exposes no HTTP port. That happens for every deployment
without the `web` interface adapter: the HTTP port is only added to the messaging
Service when `web` is enabled, but the files API is served over that HTTP port.

The web client polls `/files/usage` every 60s (storage-capacity banner) on the
chat and monitor pages regardless of whether the deployment supports files. For a
non-`web` deployment that produced a steady stream of 503s — enough, on a
low-traffic route, to trip the per-route 5xx alert (`AstroServerHigh5xxRateByRoute`)
from a single user with the page open.

## Design

The condition is a permanent configuration gap, not a transient outage, so it
should not be reported as a server fault:

- **Server** — `messagingHTTPPort` now returns a sentinel error
  (`errMessagingNoHTTPPort`) when no usable HTTP port exists. `forwardFiles`
  distinguishes it from genuine transient failures (cluster client, Service GET,
  transport) and answers **404** ("file storage is not enabled for this
  deployment") instead of 503, logging at debug rather than warn. Real
  infrastructure failures still return 503.

- **Client** — `useDeploymentStorageUsage` stops its 60s poll after a 404 (a
  permanent condition) instead of re-requesting a route that will 404 forever.
  The banner already renders nothing without data, so nothing else changes.

Net effect: the files feature degrades quietly for deployments that don't support
it, the alert no longer fires on expected conditions, and 5xx is reserved for
actual server faults.

## Migration

None. No API contract or configuration changes for users.
