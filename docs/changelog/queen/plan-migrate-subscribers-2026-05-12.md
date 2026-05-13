# Queen: migrate subscribers to the latest plan version

## Summary

When a plan in OpenMeter is republished at a higher version, existing subscribers
remain on the old version forever unless something explicitly moves them. Queen
had no flow for this — the customer detail page exposed a per-customer
"Upgrade" affordance, but it used cancel-then-resubscribe rather than the proper
migrate endpoint, and there was no way to operate on subscribers in aggregate.

This change adds a plan-scoped migrate experience: from the Plans page, an
operator can see how many customers are stuck on an older active version and
move them all to the latest version in one workflow.

## Design

**Migrate endpoint.** OpenMeter exposes `POST /api/v1/subscriptions/{id}/migrate`
which atomically closes the running subscription and starts a new one against
the requested version of the same plan key. This preserves the billing anchor,
emits a single migration audit event, and (when timing allows) takes effect
immediately. We use this in place of the cancel + create dance:

```
useMigrateSubscription()
  → POST /subscriptions/{id}/migrate { targetVersion }
```

The cancel+create path on the customer detail page is left in place for now —
it's a separate change to consolidate that onto `/migrate`.

**No bulk endpoint.** OpenMeter has no `/migrate` variant that accepts a list of
subscription IDs. The dialog iterates client-side, collects per-customer errors,
and reports a final outcome. This is acceptable while the operator base is
small; for ten-thousand-subscriber plans the right shape is a River job on
astro-server (same pattern as `openmeter.backfill`) with bounded concurrency and
poll-based progress — not implemented here.

**Discovery of affected subscribers.** Rather than fan-out per-row subscription
fetches, the customers list query now passes `expand=subscriptions` so each
`Customer` arrives with its `subscriptions[]` populated. The Plans page derives
two maps from that single list:

- `latestByKey` — the highest-version `active` plan per `key`
- `customersByPlanVersion` — customers grouped by `${planKey}::${version}` of
  their current active subscription

A plan row shows the Migrate button when (a) a higher-version active plan with
the same key exists and (b) at least one customer's current subscription is on
this exact `key+version`. Clicking the button opens a dialog that lists the
affected customers, runs the migrations sequentially with progress, and
surfaces per-customer errors without aborting the batch.

The customers table also gained a Plan column (rendering `key v{version}` for
the current subscription, or `none`) using the same expanded data — no
additional requests.

## Migration

None required.
