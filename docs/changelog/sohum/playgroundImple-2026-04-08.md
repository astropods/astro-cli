## Summary

The playground was previously implemented as an in-app chat panel inside astro-client, with a server-side proxy in astro-server that forwarded SSE and REST calls to the messaging service. This approach duplicated what the messaging service already does natively. With the playground now bundled directly into the messaging binary (`astropods/messaging#24`), the right design is to open it in a new tab — no proxy, no panel, no duplicated state.

## Design

**Chat button opens a new tab.** The button on the agent detail page reads `deployment.external_urls` for an entry with `type: "messaging"` and calls `window.open(url)`. No side panel, no iframe, no client-side SSE wiring.

**`messaging_available` field added to astro-server.** `GET /api/v1/deployments/:id` now checks whether the `{agent}-messaging` K8s ClusterIP service exists and sets `messaging_available` on the response. The button is disabled when this is false or when no ingress URL exists yet (e.g. during initial provisioning).

**Server-side proxy removed.** The three `/playground/conversations` proxy routes, `MessagingBaseURLOverride` config, and all associated handler code have been deleted. The client no longer makes any API calls for chat — the messaging service handles everything same-origin.

**Redeploy button ungateed.** The Redeploy button in the Configure panel is now always enabled (previously required a dirty form).

**`SidePanel` and `DeploymentStatusBadge` extracted** as reusable components during the refactor.

## Migration

No changes required. Agents with `interfaces: web:` configured will automatically have the Chat button enabled once their deployment has an ingress hostname. The playground URL is served from the messaging container at `/`.
