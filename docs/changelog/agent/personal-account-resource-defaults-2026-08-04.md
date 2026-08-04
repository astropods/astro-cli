# Personal account resource defaults

## Summary

Agents, Blueprints, and Knowledge Stores now open on the user's personal account instead of loading every account by default. Each page remembers its own last account filter across navigation. List results enter smoothly on initial load and after a filter or page change.

## Design

Server rendering and the hydrated client share one scope rule: a bare list URL selects the personal account, explicit `account` parameters select those memberships, and `scope=all` represents an intentional all-account view. During in-app navigation, matching SSR and TanStack Query keys avoid a default-scope refetch. On a direct browser load, the server cannot read local storage; it renders the personal default before the client restores a different saved scope once after hydration.

After hydration, same-page account-scope URL changes bypass the route loader because TanStack Query already fetches the matching list key. This removes a duplicate server request and lets rapid account selections commit in order. Initial SSR, cross-page navigation, explicit refreshes, and mutation-driven revalidation still use the loader.

The bare personal-account default remains implicit UI state, so a new user with no resources still sees each page's onboarding empty state. Explicit account or all-account URL selections remain active filters and show the filter-aware empty state. Clearing filters returns to the bare personal default, and empty result sets do not render pagination controls.

All three pages reuse the page-filter local-storage layer from #1775 with separate storage scopes. Primitive navigation links include the destination's stored scope, so its loader fetches the correct account set immediately instead of rendering the personal default and replacing it after mount. An explicit URL selection replaces that page's stored selection. Search and sort remain temporary. Stored accounts that are no longer memberships are discarded, and authentication changes clear the storage through the existing page-filter reset.

All three result areas share a 150 ms opacity-and-translate entrance keyed to the resolved account scope, server search, page, and loading state. Pagination stays outside the keyed animation boundary, preserving keyboard focus while the cards or rows replay their entrance. The browser can run both properties on the compositor; there are no per-card timers or layout measurements, and reduced-motion preferences disable the animation.

## Migration

No migration is required.
