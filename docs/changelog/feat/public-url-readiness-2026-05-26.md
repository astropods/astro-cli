## Summary

Deployments could expose a public Launch URL before ingress provisioning was complete. Users saw a working Launch button while the load balancer was not yet assigned.

## Design

**Endpoint readiness on `external_urls`.** `GET /api/v1/deployments/:id` now sets `ready` and `message` on each `ServiceEndpointInfo`. Evaluation checks whether the matching ingress has an address in `status.loadBalancer.ingress`.

**Frontend Launch gating.** The Launch button stays visible when a messaging URL exists but is disabled until `ready` is true and `messaging_available` is true. The disabled tooltip uses a fixed user-facing message aligned with the API `message` field.

**CLI visibility.** `ast deploy --wait` polls the deployment detail endpoint until the messaging URL is ready. `ast agent get` prints the Launch URL and readiness status from the same endpoint.

## Migration

No changes required. Clients that ignore `ready` behave as before; updated clients gate Launch automatically.
