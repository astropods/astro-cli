# Remove blueprints page-size toggle, fix page size at 50

## Summary

The `/blueprints` page exposed a "Per page" toggle (10 / 20 / 50) and a cookie+localStorage preference for it. In practice the toggle added clutter next to the search input and rarely changed user behavior, especially on accounts with small blueprint counts where every option fit on one page. The toggle and its preference plumbing are now gone. The page always loads 50 blueprints per page, and pagination controls only appear once the total exceeds that.

## Design

`Blueprints.tsx` no longer manages a `pageSize` state, the `useLayoutEffect` that read it from localStorage, or the cookie-driven loader value. The loader hardcodes `{ limit: BLUEPRINT_LIST_DEFAULT_PAGE_SIZE, offset: 0 }` for its first-page fetch and the component passes the same constant into the query and pagination. The `BlueprintPageSizeControl` component, the `blueprint-page-size-preference` helper, and the `BLUEPRINT_PAGE_SIZE_OPTIONS` / `BlueprintPageSize` exports have been deleted. The pagination nav's existing `totalPages > 1` self-hide now naturally means "no pagination chrome under 51 blueprints," and the toolbar is just a search input.

Changing pages also scrolls the viewport back to the top (smooth), so the user lands on the first row of the new page instead of staying anchored at the bottom where they clicked.

## Migration

None.
