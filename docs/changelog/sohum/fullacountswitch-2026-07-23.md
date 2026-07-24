## Summary

Blueprints, Knowledge Stores, and Agents now show resources from every account the user belongs to. Their account switcher is a lightweight page filter instead of a global read-scope change.

## Design

The existing `account_members` projection determines the accounts returned with the authenticated user. The server's `/me/blueprints`, `/me/knowledge`, and `/me/deployments` aggregate API from [#1728](https://github.com/astropods/astro/pull/1728) reads resources across that membership set with bounded concurrency and returns data grouped by account.

The account filter is a multi-select built on the shared control primitive. No selection queries all memberships; selecting one or more accounts constrains the aggregate read, catalog page walks, client search, and display to those accounts. Its selection is stored in repeatable `?account=` parameters so filtered links survive reloads and remain shareable. Stale or foreign account values are removed after memberships resolve.

Each page owns one aggregate TanStack Query and performs a small response merge for display. Blueprint, Knowledge, and Deployment queries walk the API's bounded per-account pages inside that query, preserving complete catalogs without an unbounded response. A newly selected account combination is hydrated from any fresh cached superset, so toggling filters does not refetch catalogs that are already local; stale or invalidated data still refreshes normally. Pagination follows the server's explicit `has_more` value and treats a full page as the compatibility signal to continue when that field is absent. Deployment pages de-duplicate by ID and stop if a response adds no new rows, preventing a misbehaving page source from looping or duplicating cards. Reaching the client safety cap preserves the catalog loaded so far instead of presenting it as a failed account. Failed-account retries use a targeted aggregate request, while permanently rejected account names are not retried. Resource mutations invalidate both their account-scoped and aggregate lists. Blueprints search and pagination run over the merged list. Knowledge and deployment results carry their owning account so navigation resolves the correct account without changing global state.

Insights remains a single-account view because its consolidated report is account-scoped. Taylor's account control is embedded in the page title, uses the shared selector and selected-item checkmark, and stores that view scope in `?account=`; selecting it refetches Insights for that account without rotating the WorkOS session or changing the default account for writes. The legacy page-level session switcher is removed now that no read surface uses it; existing write-scope behavior remains intact.

Create flows keep account choice separate from read filters. Blueprint and Knowledge Store setup expose the same **Create in** picker, and a successful create records that account as the next create default. Knowledge detail navigation carries the selected account in the URL instead of relying on a hidden global view scope.

Partial account failures keep successful accounts visible, identify the accounts with missing data, and offer a targeted retry. If a later catalog page fails, that account is withheld so search never presents a truncated catalog as complete. A total aggregate failure renders a dedicated retryable error instead of a blank body or onboarding empty state.

Filtered-empty states stay distinct from genuine onboarding empties and offer a single action to clear search and account filters. Agent sort and account controls stack at narrow widths so neither label is truncated.

The Blueprint catalog walker follows every bounded aggregate page, so client-side search and numbered pagination are not capped at the first 100 rows. Pagination clamps to the last available page when a background refresh shrinks the merged result.

Knowledge and deployment polling uses one targeted aggregate request containing only accounts with transitional rows, then stops when those rows settle. Deployment lists remain DB-only, while dashboard observability summaries read the periodically refreshed Redis cache, so cross-account pages do not multiply live Kubernetes reads.

## Migration

No action required.
