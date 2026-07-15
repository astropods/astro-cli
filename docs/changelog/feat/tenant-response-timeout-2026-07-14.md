# Configurable ingress response timeout

## Summary

Agents fronted by the tenant-router (Contour) data plane were subject to
Envoy's stock 15s per-route response timeout, which silently cut off any
agent that took longer than 15s to produce a complete response. There was no
way to raise it. This adds a per-deployment response-timeout knob to the
advanced deploy config, defaulting to 15s and overridable up to 2m.

## Design

The timeout is a deploy-time provisioning knob, layered onto the existing
"advanced" compute/volume flow rather than the authored `astropods.yml` spec —
it is an operational property of a deployment, not of the agent artifact.

- **Spec**: `DeploymentAgent.ResponseTimeout` (deployment/v1) carries the value;
  exposed to clients via `ComponentProvisioning.response_timeout`. Default and
  cap live in astro-spec as `DefaultResponseTimeout` (15s) and
  `MaxResponseTimeout` (2m).
- **Shaping/validation**: `ShapeTemplate` applies the override onto the agent,
  defaults it when unset, and validates it (must parse as a Go duration, be
  positive, and not exceed the cap) on the same path as compute/volume. The
  resolved value is echoed back so the UI shows the effective timeout.
- **Applier**: the value threads through `IngressConfig` into every tenant
  Ingress (agent frontend, messaging, ingestion webhook) as the
  `projectcontour.io/response-timeout` annotation — the only mechanism Contour
  exposes, since there is no global route-timeout setting. The single
  agent-defaults choke point in the applier backfills 15s for specs persisted
  before the field existed, so redeploys of old deployments still get a value.
- **UI**: a duration input in the "Advanced sizing" panel, with the 15s default
  as placeholder and the 2m cap surfaced in helper text.

Enforcement of the cap follows the existing provisioning pattern — validated at
template shaping and enforced by the client, which does not deploy an invalid
template.

## Migration

None. Existing deployments keep behaving as before (now with an explicit 15s
annotation instead of Envoy's implicit 15s default); no user action required.
