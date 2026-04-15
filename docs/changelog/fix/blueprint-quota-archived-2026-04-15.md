## Summary

Two bugs combined to let archived blueprints count against the quota and allowed users to exceed the blueprint limit:

1. The periodic heartbeat that syncs blueprint counts to OpenMeter was not filtering out archived blueprints, so every 5 minutes it would overwrite the correct inline count with one that included archived agents.

2. The blueprint creation endpoint had no entitlement check, so the quota was never enforced at creation time regardless of how many blueprints an account had.

## Design

**Bug 1 — heartbeat query:** `emitActiveAgents` in `heartbeat.go` now adds `WHERE archived_at IS NULL` to match the inline `EmitActiveAgents` function in `events.go`, which already filtered correctly. Both code paths now agree on what "active agent" means.

**Bug 2 — missing quota gate:** The `POST /api/v1/agents/:account` route is now wrapped with `ent.Wrap(..., "agents")`, consistent with how `RegisterAgent` and other quota-gated routes are protected. Attempts to create a blueprint beyond the account's entitlement will receive HTTP 402.

## Migration

No action required. Accounts currently over quota due to the missing gate will be blocked from creating further blueprints until they archive enough to fall within their limit.
