# Fix duplicate URL in agent pod Domains section

## Summary

When an agent has a frontend component, the agent pod's Domains panel showed
three URLs instead of two — one duplicated. The agent pod's own service URL
appears on both the deployment-level external URLs (alongside the frontend's
URL) and on the workload's own service endpoints, and the merge concatenated
without deduping.

## Design

In `PodDetailPanel.GeneralTab`, the URL list for the General tab combines:

- `deployment.external_urls` (passed in as `externalUrls`) — Ingress-level
  URLs for the whole deployment. When a frontend component exists, this list
  has entries for both the agent and the frontend.
- `workload.urls` — the workload's own service endpoints. For the agent pod,
  this includes the same external URL the deployment exposes for it.

The combination is gated on `workload.component === "agent"` for the
external URLs, but otherwise concatenated as-is. With a frontend present, the
agent pod renders `[agent_url (from external), frontend_url (from external),
agent_url (from workload)]` — the agent URL appears twice.

### Fix

Dedupe by URL string while preserving insertion order (external URLs first,
then workload URLs):

```ts
const byUrl = new Map<string, ServiceEndpointInfo>();
const add = (entry: ServiceEndpointInfo) => {
  if (!byUrl.has(entry.url)) byUrl.set(entry.url, entry);
};
if (workload.component === "agent" && externalUrls?.length) {
  externalUrls.forEach(add);
}
workload.urls?.forEach(add);
return Array.from(byUrl.values());
```

The frontend pod is unaffected: it doesn't include `externalUrls`, so its
list comes only from `workload.urls`.

## Migration

None. UI fix only.
