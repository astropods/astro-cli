## Summary

Two metering gaps are fixed: GitHub-triggered builds were not emitting `agent_build` or `active_agents` events, and creating an agent blueprint via the web UI was not emitting `active_agents`. In both cases the agent count was only reconciled by the 5-minute heartbeat rather than updated immediately. Additionally, all inline emit functions now log success, making them observable in the same way as the heartbeat.

## Design

**GitHub CI builds:** The CLI registration path already emitted both events immediately after `Register()`. The `GitHubBuildWorker` called the same `agentIndex.Register()` but had no OpenMeter wiring. The fix adds `omClient` and `db` to the worker and fires the same two events after a successful registration.

**Blueprint creation:** `CreateBlueprint` (web UI) inserts a row into `agents` without a build, so the agent count changes but no build event applies. `RegisterAgent` and `ArchiveAgent` already emitted `active_agents` inline; `CreateBlueprint` was missing it. The fix adds `omClient` and `db` to `CreateBlueprint` and fires `EmitActiveAgents` after a successful create, matching the other two handlers.

**Success logging:** The inline `Emit*` functions in `events.go` previously only logged on error. Each now logs at `Info` level on success with the same fields as the heartbeat, so inline fires are visible in logs without waiting for the next tick.

## Migration

No action required.
