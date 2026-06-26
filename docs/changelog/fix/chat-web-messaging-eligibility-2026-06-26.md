## Summary

Agents without the web chat adapter (e.g. Slack-only agents) were appearing in the chat page's agent selector and were treated as chat-eligible. They should not be selectable for web chat.

## Design

Chat eligibility is driven by the server's `messaging_web_configured` flag on the deployment summary. That flag was computed by checking whether the messaging sidecar had an `http` service — but every messaging sidecar exposes an `http` service on port 8080 (the platform messaging API the proxy talks to), regardless of which user-facing adapters are configured. A Slack-only agent has that service too, so the check flagged every messaging agent as web-configured.

`GetMessagingWebConfigured` now keys off the authoritative signal instead: the stored deployment spec's `interfaces.adapters` containing `"web"`. This matches what actually gates the web chat surface, so Slack-only and custom-only agents are correctly excluded from the chat list, the agent switcher, and the send-eligibility gate, while web (and web+slack) agents remain eligible.

## Migration

No action required.
