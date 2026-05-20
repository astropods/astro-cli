# Blueprints page search and pagination

## Summary

The `/blueprints` page listed every blueprint for the active account with no way to narrow results or load large catalogs incrementally.

## Design

Blueprints-page-only client wiring for the server list API:

- `blueprint-list-params.ts` — `BlueprintListParams`, query serialization, and filter helpers (no shared deployment types).
- `useBlueprintSearch` — debounced `q` param for the page (lives under `pages/blueprints/`).
- `useAccountBlueprintsInfinite` — paginated `useInfiniteQuery` with user-selected `limit` (10 / 20 / 50) and `has_more`.
- `useAccountBlueprints` — unchanged call sites (profile, deployment history) fetch up to 100 items via default API merge.

The page shows a compact search field, per-page size tiles, “Showing N of M”, and Load more when `has_more` is true.

**Follow-up:** Sync search and page size with URL search params (`useSearchParams`) so filter state survives navigation and can be shared.

## Migration

None for other pages. `/blueprints` now pages at 50 items; use Load more or search to reach the rest.
