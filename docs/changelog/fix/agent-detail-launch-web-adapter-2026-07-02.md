# Agent detail Launch button — gate on web messaging adapter

## Summary

The Launch button on the agent detail page appeared for any agent with a messaging sidecar, even when the web (chat) adapter was not enabled. Launch deep-links into the in-app web chat, so on a slack-only agent it rendered a dead action that routed to a surface the agent cannot serve. The agents list and chat surfaces already gate on the web adapter; the detail page did not.

## Design

The detail page now uses the same eligibility signal as the agents list and chat list: `messaging_web_configured`, checked through the shared `isChatListEligible` helper.

- The detail record endpoint only carries `messaging_configured` (true whenever a messaging sidecar exists, including slack-only agents), which is not a reliable signal for the web chat. The authoritative flag `messaging_web_configured` rides on the deployments list summary.
- The page reads that summary from the already-cached deployments list query (`useDeployments(account)`) and finds the current deployment by id, rather than adding a new server field to the record endpoint.
- Launch renders only when the summary confirms `messaging_web_configured === true`. While the list is still loading, or when the web adapter is absent, the button is not rendered — it can only transition hidden → shown, never shown → hidden, so it never flashes out.

## Migration

None required.
