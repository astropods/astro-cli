# Scope filter updates and knowledge store UI consistency

## Summary

Brings the knowledge stores UI in line with the established patterns used by agents and blueprints — scope switching, breadcrumbs, page backgrounds, and empty states. Also removes the unused "default view" star from the scope switcher.

## Design

**Scope switcher on knowledge stores** — The listing page now uses `PageContainer` + `PageHeader` with `PageScopeSwitcher` in the adornment slot, matching the agents and blueprints pages.

**Shared breadcrumbs** — Knowledge store detail and new store pages replace hand-rolled breadcrumb markup with the shared `PageBreadcrumb` component. Both support mobile collapse to an account avatar, matching the blueprint detail pattern. The new store breadcrumb appends a provider sub-label (e.g. "Add store / PostgreSQL") when a provider is selected.

**Breadcrumb refinements** — Letter spacing tightened at the component level (`tracking-normal`) to reduce the wide feel from `text-mono-sm`. Mobile gets `leading-normal` to fix vertical centering when avatars are present.

**Default view removal** — The star icon for setting a default account is removed from `OrgSwitcher`. The `defaultAccount` and `toggleDefault` are dropped from the active account context. The underlying `useDefaultAccount` hook and localStorage persistence remain intact for account selection.

**Visual consistency** — Page backgrounds updated to `bg-stone-100` on agents, blueprints, explore, and knowledge stores. Empty states aligned between agents and stores (icon container, button sizes, label text).

## Migration

No migration required.
