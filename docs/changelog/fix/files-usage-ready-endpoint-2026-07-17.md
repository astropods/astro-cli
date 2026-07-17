# Files API: 404 when a non-web agent has no messaging Service

## Summary

`AstroServerHigh5xxRateByRoute` kept firing on `/deployments/:id/files/usage`
after the event-driven-poll fix (#1657). Root cause, from prod logs:

```
files proxy target resolution failed  get messaging service
"gong-fde-pipeline-messaging": services "gong-fde-pipeline-messaging" not found
Server error  path=/api/v1/deployments/.../files/usage  status=503
```

The deployment is a non-web (pipeline/cron) agent. It has **no messaging
Service at all**, so `resolveMessagingProxyTarget`'s `Services().Get()` returns
NotFound and `forwardFiles` fell through to its generic 503 ("files endpoint
unavailable"). Since the agent is `active` (its cron workload runs), neither the
DB-status guard (#1657) nor the no-http-port guard (#1656) applied — the former
only catches non-active deployments, the latter only a Service that exists but
lacks an http port. A missing Service slipped between them.

## Design

A missing messaging Service is the same class of condition as one without an
http port: the deployment has no files endpoint. It's permanent and expected,
not a server fault, so it should read as 404.

`forwardFiles` now treats a NotFound resolve error the same as
`errMessagingNoHTTPPort` — both answer **404** ("file storage is not enabled for
this deployment"), logged at debug. The classification lives in a small
`filesEndpointAbsent` helper. Genuine transient failures (cluster client,
transport, an existing sidecar returning 5xx) still surface as 5xx.

No new Kubernetes calls: this reuses the `Services().Get()` the resolve path
already performs — it only reclassifies its error.

**Client — stop making the call.** The storage-capacity banner gated its
`/files/usage` fetch on `status === "active"`, but a pipeline agent is active,
so it still fetched. It now gates on `DeploymentRuntime.messaging_reachable`
(from the runtime endpoint the detail pages already load) — the live signal for
"the sidecar that serves this reading can answer." It's `false` when the
messaging Service is absent or the sidecar isn't ready, so the request is never
made. The server 404 remains the backstop for any other caller.

## Migration

None. No API contract or configuration changes for users.
