## Summary

Cross-account resource pages can load Blueprints, Knowledge Stores, and Deployments through authenticated aggregate reads instead of issuing an unbounded request fan-out from the browser. This keeps latency and database pressure predictable for users who belong to many accounts.

## Design

The API exposes separate `/me/blueprints`, `/me/knowledge`, and `/me/deployments` aggregate reads so each page fetches only the resource it renders. The user-centric namespace reflects that these collections contain resources visible to the authenticated user rather than resources owned by a synthetic "all accounts" scope. The authenticated user's `account_members` projection defines the readable account set; callers may narrow a request with repeated `account` parameters. Requested names outside the current membership set are reported separately from transient account-load failures, so clients do not retry permanently unauthorized names.

Each response keeps data grouped by account and reports account-level failures separately. A failure in one account therefore does not hide successful data from other accounts, and clients can retry only the failed accounts. Server-side work is capped at six concurrent accounts.

Blueprint, Knowledge, and Deployment aggregation use the same bounded `limit`/`offset` contract as the existing Blueprint list (50 rows by default, 100 maximum per account). Timestamp ordering uses the resource ID as a unique tiebreaker so rows cannot move between adjacent pages when timestamps match. Every account result explicitly reports its count, limit, offset, and whether more rows remain, including requests beyond the final page, zero values, and terminal `has_more: false`; Blueprint and Deployment results retain their existing list envelopes. Clients that need a complete catalog can walk bounded aggregate pages without issuing one request per account. Deployment aggregation slices the existing invalidated per-account read-through cache when available and falls back to a bounded DB-only enrichment path without introducing Kubernetes reads. The original single-account deployment list serves both cache hits and newly cached misses as raw JSON bytes, keeping its frequently polled response path consistent and avoiding decode/re-encode work.

The aggregate shapes retain the existing per-account list envelopes inside each result. Clients can continue populating the established TanStack Query keys so detail pages and mutation invalidation share the same cache.

## Migration

No user action is required. Clients can adopt the aggregate endpoints independently for each resource page.
