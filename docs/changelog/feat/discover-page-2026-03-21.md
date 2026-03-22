# Rename Browse to Blueprints with Account-Scoped Sidebar

## Summary

The Browse page has been renamed to Blueprints and restructured around account-scoped sections instead of tag-based categories. The route changes from `/browse` to `/blueprints` with nested sub-routes for each section.

## Design

The page follows the same layout + `<Outlet />` pattern as the Settings page. Each sidebar section is a real route:

- `/blueprints/discover` — all public blueprints system-wide.
- `/blueprints/personal` — the authenticated user's blueprints (public and private), with their avatar in the sidebar.
- `/blueprints/:account` — an organization's blueprints, one sidebar entry per org.

Unauthenticated users see only the Discover section. `/blueprints` redirects to `/blueprints/discover`.

All three routes have SSR loaders that prefetch agent data so the page renders pre-populated. The Personal loader resolves the personal account name via `getProfile()` and passes it through `loaderData` to avoid re-deriving it client-side.

Shared presentation lives in `AgentListView`, a component that handles loading, error, empty, and grid states. Both `Discover` and `AccountAgentsList` delegate to it, differing only in which query hook they call.

Route paths are centralized in `blueprintsPaths` (in `lib/routes.ts`) to avoid hardcoded strings across the sidebar, redirects, and page components.

## Migration

The route changed from `/browse` to `/blueprints`. All internal links have been updated. No API changes required.
