# Fix: Deployment limit error shows `[object Object]`

## Summary

When a user tried to deploy an agent after reaching their maximum number of active deployments, the UI displayed `Deployment failed\n[object Object]` instead of a meaningful message.

## Design

The server's entitlement limit response includes an `error` string (the human-readable message) and a `details` object (structured metadata). The client's error extraction in `useDeployForm` preferred `details` over `error`, but `details` is a plain object — not a string — so it serialized as `[object Object]`.

The fix checks `error` first, then falls back to `details` only when it is a string.

## Migration

No action required.
