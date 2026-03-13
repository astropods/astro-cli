# Agent Hearts

## Summary

Adds a "heart" (star) system for agents — users can heart/unheart agents to signal interest. Heart counts are returned inline with agent list and detail responses so the frontend can display them without extra API calls.

## Design

**Database:** New `agent_hearts` table with composite primary key `(account_id, agent_name, user_id)` enforcing one heart per user per agent. Foreign key cascades to `agents` so hearts are cleaned up on agent deletion.

**Store (`heartstore`):** Three query methods — `Toggle` atomically inserts or deletes a heart in a single CTE and returns the new state + count, `Info` returns count + user status for detail pages, and `BulkCount` returns per-agent counts for an account to avoid N+1 on list pages.

**API:** A single `POST /agents/:account/:name/heart` toggle endpoint (auth required, idempotent). Returns `{ hearted, heart_count }`. The existing `GET /agents`, `GET /agents/:account`, and `GET /agents/:account/:name` responses now include `heart_count` and `hearted` fields populated via the store's bulk and info queries.

**Frontend:** `Agent` type extended with optional `heart_count`/`hearted`. A `useToggleHeart` mutation hook calls the toggle endpoint and optimistically patches the TanStack Query cache (detail + both list queries) on success instead of refetching.

## Migration

Atlas will auto-diff the new `agent_hearts` table and index from `schema.sql`. No data migration needed — the table starts empty.
