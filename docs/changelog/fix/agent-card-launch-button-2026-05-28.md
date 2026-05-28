# Agents-page card: show Launch button whenever messaging URL exists

## Summary

The Launch button was hidden on cards whose messaging endpoint existed but wasn't yet flagged ready — typically during the seconds-to-minutes window while the ALB controller stamps the Ingress status. The URL itself is deterministic from agent name + namespace + domain and is correct from the moment the deploy applies, so hiding the button was misleading.

## Design

The messaging URL written into `Ingress.Spec.Rules[].Host` is final the moment the deploy applies — it doesn't depend on the messaging container booting. What lags is the ALB controller's update to `Ingress.Status.LoadBalancer` (which drives our `ready` flag). Since the URL is stable across that transition, the simplest behavior is to render the Launch button whenever a messaging URL is present and let users click through; the cloud edge handles the not-yet-routable case.

`DeploymentAgentCard` now passes `messaging?.url` directly as `launchUrl`. The card renders Launch whenever that URL is set. No more `isLaunchReady` gate on the grid.

## Migration

None.
