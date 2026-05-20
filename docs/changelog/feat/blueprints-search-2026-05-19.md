# List search — server API (blueprints)

## Summary

Blueprint list endpoints returned full collections with no way to filter or page results. UIs had to download everything and filter in the browser.

## Design

Optional query parameters on blueprint list routes; filtering and pagination run in PostgreSQL.

**Blueprints** (`GET /api/v1/agents`, `GET /api/v1/agents/:account`):

- `q` — ILIKE on agent name, latest version description, and tags (jsonb)
- `tag` — exact match on latest version `agent_card` tags
- `visibility` — `public` or `private` (account list only; public catalog remains public-only)
- `sort` — `name` (default) or `newest`
- `limit` — page size (default 50, max 100)
- `offset` — zero-based offset (default 0, max 10_000)

**Response shape:**

```json
{
  "agents": [],
  "count": 42,
  "limit": 50,
  "offset": 0,
  "has_more": true
}
```

`count` is the total matching rows before pagination. `has_more` is true when `offset + len(agents) < count`.

Paginated list queries (`limit` > 0) read `count` from `COUNT(*) OVER()` in the same SELECT as the page rows, so total and page data are consistent. Unpaginated internal callers (`limit` = 0, e.g. GetProfile agent summaries) skip the count query and set `Total` to `len(agents)` after the scan.

Shared parsing lives in `handlers/list_filters.go`. SQL filters live in `internal/agentindex` (`BlueprintListOptions`, `BlueprintListPage`). Invalid enum values return 400.

**Hardening:** Non-members listing `GET /api/v1/agents/:account` always query `visibility = 'public'` in SQL so `count` / `has_more` cannot reveal private catalog metadata. List handler 500s omit DB error details. `q` / `tag` use parameterized ILIKE with `ESCAPE`; `limit` and `offset` are bounded.

Deployment list filtering is deferred to a follow-up PR.

**Follow-up:** `internal/agentindex/blueprint_list_query.go` builds dynamic SQL with string concatenation and `#nosec G202` to satisfy gosec. Elsewhere in astro-server (e.g. `openmeter/billing.go`) the usual pattern is `fmt.Sprintf` plus `//nolint:gosec` with a short comment that `$N` placeholders are parameterized. Align blueprint list query assembly with that style in a small cleanup PR when convenient.

## Migration

Clients that assumed `count` was the page length should use `count` as total matches and `has_more` for infinite scroll. Omitting filter params still returns the first page (limit 50) instead of the full list — increase `limit` or paginate with `offset` if the full catalog is needed.
