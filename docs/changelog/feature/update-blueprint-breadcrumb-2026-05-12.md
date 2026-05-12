# Blueprint Detail Breadcrumb Updates

## Summary

Refines the blueprint detail breadcrumb so the account crumb deep-links to that account's profile and so the root crumb reflects the surface the user came from.

## Design

- **Account crumb → profile page.** The middle breadcrumb item now links to `accountProfilePath(account)` (`/:account`) instead of the filtered Blueprints list (`/blueprints?account=…`). This matches the pattern already used by the knowledge breadcrumbs and lets users navigate up to the publisher's home rather than a filtered search.
- **Root crumb mirrors the referrer.** `BlueprintCard` accepts an optional `from` prop and seeds outgoing router state with `{ from }`. `BlueprintDetailBreadcrumb` reads `useLocation().state.from` and, when the referrer starts with `/explore`, renders the root crumb as **Explore → /explore**. Anything else (including direct loads where `state` is `null`) falls back to **Blueprints → /blueprints**.
- **Origin lifted to the page level.** `BlueprintListView` forwards a `from` prop to each card; pages decide the value statically — `Explore` passes `from={explorePath}`, the community-blueprint cards inside the Blueprints empty state pass the same. Surfaces that should default to "Blueprints" (profile tabs, recommended-agent rails, the populated `/blueprints` page) simply omit it. The card never calls `useLocation`, so a query-param change on `/explore` or `/blueprints` doesn't re-render every card in the grid.
- **Test utilities.** `WrapperOptions.initialEntries` was widened from `string[]` to `InitialEntry[]` so tests can seed `MemoryRouter` with `{ pathname, state }` and exercise referrer-based behavior.

## Migration

No migration required.
