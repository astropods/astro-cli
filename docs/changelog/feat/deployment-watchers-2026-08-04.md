# Route deployment alerts to watchers

## Summary

Deployment alerts had no notion of who cares about a given deployment. Observation alerts (crash loops, OOM kills, over-provisioning) resolved to an account-wide audience, so in an organization every member — or, after the preceding change, every manager — was mailed about every agent regardless of whether they had ever touched it. The people most able to act on an alert are the ones who deployed or reconfigured the thing, and they were indistinguishable from everyone else.

Watchers make that relationship explicit. A member becomes a watcher of a deployment by acting on it, and deployment alerts resolve to that deployment's watchers.

## Design

**Membership is implicit, and derived from the audit trail.** Rather than add a "subscribe" call to each deployment handler, enrollment hangs off the audit-log seam. Every deployment mutation already writes an audit row carrying actor, actor type, resource type, and resource id — everything a subscription needs. A new `auditlog.Observer` hook runs after a successful audit write, and the watcher observer turns qualifying events into rows. The enrollment policy is a single predicate: user actors only, `resource_type = deployment`, action prefixed `deployment.`.

This is the central design decision. Instrumenting handlers one at a time is how call sites get missed, and the miss is silent — someone simply never gets alerted. Deriving from the audit trail means any action already recorded there is covered, and a future audited action is covered without a second edit. Admin and system actors are excluded deliberately: operators and automation act on deployments across many accounts, and enrolling them would mail them everything.

Observers are advisory. The audit row is the contract; a failed enrollment is logged and does not fail or undo the action the member just performed.

**Routing is a new audience with a mandatory fallback.** `notify.AudienceWatchers` resolves recipients from the deployment's unmuted watchers. It is scoped by a new `Event.DeploymentID`, kept distinct from `EntityID` — `EntityID` is the Novu dedupe key and is not always a deployment (`build.failed` carries a build id), so overloading it would force routing to guess what the field currently means.

Every path that yields no usable recipient falls back to the account's managers: no watcher lookup wired, no deployment scope on the event, nobody watching yet, the lookup erroring, or every watcher lacking a mirrored email. An alert with no watchers is a routing gap, not a reason to stay silent. Because there is no backfill, existing deployments take this path until members act on them — so behavior is unchanged at rollout and the audience narrows gradually as watchers accrue.

`build.failed` stays manager-addressed. A build can fail before any deployment exists, so there is no deployment to resolve watchers from; the right vector there is agent-level watchers, which this change does not build.

**Opting out is a sticky mute, not a delete.** Removing the row would let the member's next deploy silently resubscribe them to alerts they had just turned off. Instead the row persists with `muted` set, and enrollment never clears that flag. Muting also upserts, so opting out of a deployment you have never touched sticks and your first deploy does not enroll you.

## Migration

None required. A new `deployment_watchers` table is added and starts empty; deployments with no watchers fall back to the manager audience, which is the behavior that was already in place. Three endpoints are added — `GET /deployments/:id/watchers` and `POST`/`DELETE /deployments/:id/watchers/me` — with no client UI yet.
