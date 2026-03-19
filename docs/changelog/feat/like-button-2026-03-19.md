# Wire up agent like button

## Summary

The like button on agent detail pages was purely cosmetic — clicking it toggled local state but never called the API. This change connects it to the existing `POST /agents/:account/:name/heart` endpoint and fixes a server-side bug that returned stale heart counts.

## Design

**Frontend** — The `AgentDetailBreadcrumb` component now uses the `useToggleHeart` mutation hook with TanStack Query optimistic updates. On click, the cache is immediately patched (heart state + count), giving instant feedback. On error, the cache rolls back to a snapshot taken before the mutation. On settle (success or failure), affected queries are invalidated to reconcile with the server.

The button displays a GitHub-style "Likes | N" format with a persistent counter.

**Backend** — The `heartstore.Toggle` SQL query used a `COUNT(*)` in the main SELECT alongside data-modifying CTEs. Due to PostgreSQL's CTE snapshot isolation, the count reflected the pre-mutation state — liking returned the old count, unliking returned a count that was too high. Fixed by computing the count as `prev_count ± 1` based on which CTE (insert or delete) executed.

## Migration

No migration required. The `agent_hearts` table and API endpoint already exist.
