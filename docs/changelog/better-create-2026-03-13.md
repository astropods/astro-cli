# Agent Hearts

## Summary

Adds a "heart" system for agents — users can heart/unheart agents to signal interest, similar to GitHub stars. Heart counts are returned inline with agent responses so the frontend can display them without extra API calls.

## Design

**Database:** New `agent_hearts` table with a composite primary key `(account_id, agent_name, user_id)` enforcing one heart per user per agent. Foreign key cascades to `agents` so hearts are cleaned up when an agent is deleted.

**Store (`heartstore`):** Provides single-agent operations (`Heart`, `Unheart`, `Info`) and batch operations (`BulkCount`, `BulkIsHearted`) to avoid N+1 queries on list pages. `Info` combines count + user status in a single query.

**API changes:**
- `GET /agents`, `GET /agents/:account`, `GET /agents/:account/:name` — responses now include `heart_count` and `hearted` fields. List endpoints use `BulkCount` per account; detail endpoint uses `Info`.
- `PUT /agents/:account/:name/heart` — heart an agent (auth required, idempotent)
- `DELETE /agents/:account/:name/heart` — unheart an agent (auth required)

**Frontend:** `Agent` type extended with `heart_count`/`hearted`. API client methods and a `useToggleHeart` mutation hook added. The mutation invalidates agent queries on success.

## Migration

Atlas will auto-diff the new `agent_hearts` table and index from `schema.sql`. No data migration needed — the table starts empty.
