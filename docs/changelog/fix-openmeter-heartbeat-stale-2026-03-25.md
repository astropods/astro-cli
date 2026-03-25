# Fix: Stale OpenMeter usage allows exceeding deployment and agent limits

## Summary

Users could deploy more agents than their plan's limit (default 10) because the entitlement check at deploy time read usage from OpenMeter, which was only updated by a background heartbeat running every 5 minutes. Within that window, rapid consecutive deploys all passed the same stale check.

## Design

OpenMeter tracks `agent_deployments` and `agents` as gauge-type metered entitlements. The heartbeat reconciles these every 5 minutes by querying the database and emitting count events. The entitlement check on `POST /deploy` and `POST /agents/:account/:name/register` reads from OpenMeter, so it could be up to 5 minutes behind.

The fix mirrors the existing pattern used for team members (`EmitActiveMembers`): emit an inline count event immediately after a successful write, so the next entitlement check sees the updated value without waiting for the heartbeat.

Two new functions in `internal/openmeter/events`:
- `EmitActiveDeployments` — queries `pending + active + scaled_down` deployments for the account and emits `active_deployments`
- `EmitActiveAgents` — queries non-archived agents and emits `active_agents`

Both are fired as background goroutines after the database write succeeds. `DeployAgent` emits only on new deployments (not redeploys). The heartbeat continues to run as a periodic reconciliation.

## Migration

No migration required.
