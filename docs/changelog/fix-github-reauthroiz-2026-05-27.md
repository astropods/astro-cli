## Summary

The GitHub "Reauthorize" button in Settings → Connectors was broken: clicking it showed "Opening GitHub…" briefly then did nothing. The server was short-circuiting on a valid existing token and returning `{connected: true}` instead of a redirect URL, so the OAuth flow never started. A secondary bug caused a 500 when `force=true` and the token was valid due to an incorrect error guard.

Additionally, the connector page had no guidance for users wanting to grant access to additional GitHub organizations, and rendered a spurious empty border element when not connected.

## Design

A `force` boolean field is added to `GitHubAccountConnectRequest`. When `force=true` (sent only by the Reauthorize button), the server skips the existing-token check entirely and proceeds directly to `GetAuthorizationURL`, always returning a redirect URL. The Connect GitHub button and all other call sites (`NewBlueprint`, `GitHubConnectionPanel`) do not set `force`, preserving the short-circuit for the initial connect flow.

The error guard was tightened to `err != nil && !errors.Is(...)` so a nil error on the `force=true` path no longer falls through to a 500.

On the client, `handleConnect` and `handleReauthorize` are separate handlers sharing an `onConnectSuccess` callback. The connector card now shows a "Request access on GitHub" link (pointing to `github.com/settings/connections/applications`) in both the zero-orgs and has-orgs states, since GitHub OAuth re-auth does not re-show the org selection screen for already-authorized apps — org access must be managed directly in GitHub settings.

The empty `<ul>` rendering when disconnected was caused by four separate `{connected && ...}` expressions producing `[false, false, false, false]` as children — a truthy array — which passed the `ConnectorCard` child guard. Wrapping all children in a single `{connected && <>...</>}` collapses to a single `false` when disconnected.

## Migration

No action required.
