## Summary

Settings pages were using a one-off div-based grid pattern (`GRID_COLS` constant + manually bordered rows) to render tabular data. This predated the shared `Table` component now used on the Knowledge Stores page and in Storybook. The goal is visual consistency across all tables in the platform.

## Design

All four settings tables — Members, Variables & Secrets, Audit Log, and Quota Requests (Usage) — are migrated to the shared `Table` / `TableHeader` / `TableRow` / `TableHead` / `TableBody` / `TableCell` primitives from `@/components/ui/table`.

The `isLast` prop pattern (manually toggling `border-b` on the last row) is removed; the Table component handles this via `last:border-b-0` on `TableRow`. The `GRID_COLS` constants are also removed — column widths now derive from content, matching the pattern used on the Stores page.

`TableHead` vertical padding was tightened from `py-2.5` to `py-2` for a more compact header, applied at the component level so it's consistent everywhere.

The org-members Playwright e2e selectors were updated from `div.bg-surface > div` to `tbody tr` to match the new semantic markup.

## Migration

No user-facing changes. No action required.
