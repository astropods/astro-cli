# Agents grid: populate messaging URL in cached list payload

## Summary

The Launch button on the agents grid card was hidden for every deployment because the list endpoint that feeds the grid never returned a messaging URL — even though the card's render logic was already correct. The detail page worked because it uses a different (heavier) K8s fetcher; the grid uses a "light" variant that deliberately skipped ingresses for performance. With no `external_urls` on a deployment, `getMessagingEndpoint(deployment)?.url` is undefined and the card falls back to a full-width "Manage agent" button.

## Design

`deploycache` memoizes the JSON payload that `ListDeployments` produces. Its contract is schemaless — it stores whatever bytes the handler hands it — so the cache faithfully kept storing a payload that lacked URLs. The fix is upstream of the cache: make the light K8s fetcher include the data the card needs, so cache writes carry the URL forward to every subsequent read.

`listAstroDeploymentsLight` now lists Ingresses alongside Deployments and StatefulSets, builds the same `agent:version → []ServiceEndpointInfo` map as the heavy variant, and assigns it onto each agent's `ExternalURLs` at seed time. Pods and jobs remain skipped — the light variant is still light. One extra `Ingresses().List()` per namespace lands only on cache miss; steady-state reads remain Redis-only because every deploy / undeploy / reconcile path already invalidates the per-account entry.

The client side is unchanged. The card's existing branch (`launchUrl ? <Launch> : <Manage>`) just starts seeing a populated URL once the cache repopulates against the new payload shape.

The `MessagingURLOverride` config (local-dev only, currently only honored by the detail endpoint) is intentionally not mirrored here; local dev surfacing for Launch can be addressed separately if needed.

## Migration

None. Cache entries written by the previous code will roll over via their existing invalidation paths.
