# Event-Driven Compute Metering

## Summary

Replaces heartbeat-primary compute metering with event-driven lifecycle events as the source of truth. A new `BillingStateManager` emits precise CU-hour CloudEvents at each billable state transition rather than assuming each active deployment ran a full heartbeat interval. New `deployment_billing_state` and `knowledge_billing_state` tables track the billing anchor timestamp per workload/store, preventing double-counting between inline events and heartbeat ticks.

## Design

**Before:** Heartbeat polls every 5 min and assumes each active deployment ran the full interval. Misses short-lived deployments and over/under-counts at boundaries.

**After:** Lifecycle events are primary (start/stop with timestamps), heartbeat is a safety net that reconciles deltas. Both paths advance `last_emitted_at` so neither double-counts.

**Billing cycle** (`RunBillingCycle` / `RunKnowledgeBillingCycle`) runs on every heartbeat tick:
1. **Heal** — insert missing billing rows for active deployments/stores (`ON CONFLICT DO NOTHING`, idempotent)
2. **Emit active** — emit delta CU-hours since `last_emitted_at` and advance the anchor
3. **Reconcile stale** — catch rows left `billing_active=true` by a crash or missed stop event
4. **Reconcile stopped** — emit the final period for rows with `stopped_at` set; clear the sentinel when done

**Key scenarios:**
- **Short-lived deployment (< 5 min):** `StartBilling` sets the anchor, `StopBilling` records `stopped_at`, reconcile-stopped emits fractional CU-hours on the next tick
- **Crash recovery:** stale `billing_active=true` row is detected, billed up to `status_changed_at`, then deactivated
- **Emission failure:** `last_emitted_at` and `stopped_at` are not advanced on error — the row is retried on the next tick

**Knowledge store billing** follows the same pattern. `StopKnowledgeBilling` records `stopped_at` without emitting; the heartbeat emits the final period and then deletes the billing row. The row intentionally outlives the `knowledge_stores` row (no CASCADE) so the final period can be emitted after the store is gone.

The timestamp advance query uses a subquery (`WHERE deployment_id IN (SELECT ...)`) rather than `UPDATE ... FROM` for compatibility with both SQLite (unit tests) and PostgreSQL (production).

## Migration

No action required. Schema tables are created automatically on server startup. Deployments active before this change will be picked up on the next heartbeat tick; `last_emitted_at` is initialized to that tick rather than deploy time, so a short window at the boundary may not be billed.

## Test Plan

- [ ] Deploy an agent and confirm it reaches `active` status
- [ ] After ~1 min, confirm a `compute_usage` event appears in OpenMeter with a non-zero `compute_unit_hours`
- [ ] Undeploy the agent and confirm a final `compute_usage` event is emitted on the next heartbeat tick
- [ ] Confirm subsequent heartbeat ticks emit nothing for the undeployed agent
- [ ] Deploy an agent, kill the server while it is active, restart, and confirm the next heartbeat emits a catch-up event and deactivates the billing row
- [ ] Create a knowledge store and confirm `knowledge_compute_usage` events appear after ingestion completes
- [ ] Delete the knowledge store and confirm a final `knowledge_compute_usage` event is emitted before deletion
