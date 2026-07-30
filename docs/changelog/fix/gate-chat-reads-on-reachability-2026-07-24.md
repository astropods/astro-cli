# Gate chat proxy reads on messaging reachability

## Summary

The chat conversation list and two agent/config reads fired at a deployment's
messaging sidecar regardless of whether that sidecar was reachable. Against a
stopped, paused, deploying, or mid-rollout deployment those proxy calls 5xx, and
a stopped deployment still serves its DB-backed status/runtime records, so the
chat page loads and issues them. The result was `AstroServerHigh5xxRateByRoute`
firing on `/chat/conversations` and `/messaging/agent/config` for deployments
nobody was running.

## Design

The runtime read-model already exposes the canonical readiness signal:
`DeploymentRuntime.messaging_reachable` (the messaging Service exists AND the
sidecar container is Ready, observed from the controller's snapshot). The
inspector's Settings tab already gated its agent/config read on it; the other
callers did not.

This extracts that gate into one shared hook, `useDeploymentChatReadiness`, and
applies it at the three ungated call sites:

- the conversation list (`useDeploymentChatConversations`, now taking an
  `enabled` flag like `useDeploymentAgentConfig` already did),
- the composer's file-capability read (`DeploymentChatRuntimeProvider`),
- the inspector's Files-tab visibility read (`ChatInspectorPanel`).

The hook returns `{ state, resolved, ready }`, where `ready` requires status and
runtime to have *settled* before trusting the derived state (a runtime read
error counts as settled, since that read is DB-backed and cluster-independent).
The Settings tab now consumes the same hook instead of duplicating the logic.

When a deployment isn't reachable the reads simply don't fire: the history list
shows empty, the upload affordance and Files tab stay hidden, and the Settings
tab shows its existing "not reachable / starting / paused" notice.

This complements the server-side backstop (the proxy routes already return 404
rather than 503 for an absent/stopped messaging endpoint); the client change
also covers the "Service present but sidecar not Ready" case, which the server
backstop cannot classify from a resolve error alone.

## Migration

None. No API contract or configuration changes.
