## Summary

Infrastructure changes that consolidate account-scope management into a single shared context and clean up the page structure ahead of the nav redesign.

## Design

**`ActiveAccountProvider`** — new React context that reads the active account from localStorage once and exposes `activeAccount` / `setActiveAccount` to the entire tree. Previously each page read localStorage independently, so scope changes in the nav weren't reflected until a re-render cycle.

**Per-page scope switchers removed** — `OrgSwitcher` instances on the Agents and Knowledge Stores pages are deleted; the single nav-level switcher is the sole control point.

**`/dashboard` → `/agents`** — the main deployed-agents route is renamed. All internal links, breadcrumbs, loader references, and tests updated accordingly.

## Migration

No migration required for users. Any deep-links to `/dashboard` will need updating if bookmarked.
