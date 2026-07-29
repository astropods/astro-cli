# Surface actionable Files API errors in chat

## Summary

Files API failures now use the same `{ error, details }` JSON contract from the
messaging sidecar through `astro-server` to `astro-client`. The chat Files panel
and attachment download chips surface actionable errors for invalid requests,
missing files, oversized uploads, full deployment storage, authorization
failures, and temporary service failures.

## Compatibility and safety

`astro-server` continues to understand legacy plain-text sidecar failures, so
the client and server can roll out before every deployment sidecar is updated.
Known errors are converted to canonical user-safe messages. Kubernetes proxy
responses and arbitrary object-store bodies are logged server-side but are not
exposed in the browser.

Batch uploads retain the error for each failed file instead of letting a later
successful upload clear it. List, download, and delete failures are also visible
at the point where the user initiated the action.

## Not-running deployments

When a deployment isn't reachable (stopped, no web adapter, or a pod that's
mid-rollout) its messaging proxy routes now return **404** rather than proxying
and forwarding the dead backend's 5xx. That keeps a not-running deployment from
tripping the per-route 5xx alert; genuine faults on a running deployment still
surface as 5xx.

This guard applies to every deployment route that proxies to the messaging
sidecar, not just files. A stopped deployment still serves its DB-backed
`/status` and `/runtime` records, so the client can load its detail and chat
pages and fire calls at the proxy routes. Two of those routes,
`/chat/conversations` and `/messaging/*` (e.g. `agent/config`), previously
returned 503 for a stopped deployment and were the last ones tripping
`AstroServerHigh5xxRateByRoute` (observed on preview when a stopped deployment's
chat page was opened). They now share the same classification as files via
`messagingEndpointAbsent`: a missing messaging Service (NotFound) or a Service
without an http port is an expected 404; anything else stays a 503. The chat and
messaging handlers also short-circuit to 404 on a non-active deployment status
before any upstream dial.

## Verification

Focused server and client tests cover the standard envelope, legacy responses,
oversized files, insufficient storage, Kubernetes failures, and untrusted
object-store responses. Added handler tests assert the chat and messaging proxy
routes return 404 without dialing upstream for a stopped deployment, plus a
`messagingEndpointAbsent` classification test shared across the proxy handlers.
