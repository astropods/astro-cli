# Mobile-friendly account profile

## Summary

The account profile page (`/:account`) was built for desktop with a hard 288px sidebar sitting beside the tab content. On narrow viewports the sidebar crowded the tabs into a sliver, the tab toolbars overflowed horizontally, and the page only scrolled inside the tab pane. This change makes the whole page responsive so it reads cleanly from phone widths up through 1500px.

## Design

**Layout flips axis at `md` (768px).** Below `md` the page stacks vertically — full-width sidebar on top, tabs and content below — with the page itself scrolling. At `md+` it reverts to the side-by-side layout with the sidebar fixed and the tab pane internally scrollable.

Key plumbing in `ProfileLayout.tsx`:

```tsx
<PageContainer className="flex flex-col px-0 pt-0 pb-0 md:flex-row md:min-h-0 md:px-8 md:pt-8 md:pb-8">
  <aside className="w-full md:w-72 md:shrink-0 border-b border-border md:border-b-0 md:border-r md:overflow-hidden">…</aside>
  <main className="relative flex flex-1 min-w-0 flex-col md:min-h-0">
    <div className="flex flex-wrap items-end gap-x-5 gap-y-2 px-4 pt-4 sm:px-6 md:px-8 md:pt-5 border-b border-border">…tabs…</div>
    <div className="flex-1 md:overflow-y-auto px-4 py-5 sm:px-6 md:px-8 md:py-6">…content…</div>
  </main>
</PageContainer>
```

Notes:
- `PageContainer`'s default `px-6` outer padding is zeroed on mobile (tab content + sidebar shells own their own padding) and restored at `md+`. The container uses `tailwind-merge` so overrides cascade cleanly.
- The aside's `overflow-hidden` and the tab-pane's `overflow-y-auto` are gated to `md:` — on mobile the page is the scroll root.
- Sidebar border flips from a right border to a bottom border so the stacked layout still has a visual divider.

**Tab bar wraps.** The tab row uses `flex-wrap`, so the "View as visitor" admin button (with `ml-auto order-last`) drops to a second row at narrow widths instead of squeezing the tabs.

**Toolbars wrap consistently.** All three tab toolbars (`BlueprintsTab`, `AgentsTab`, `HeartsTab`) use `flex flex-wrap items-center gap-x-3 gap-y-3` so search + filter dropdowns + secondary actions stack cleanly when they don't fit on one row. The Agents tab's "Only visible to you" tag uses `sm:ml-auto` so it sits inline at `sm+` and stacks under the controls below that.

**Sidebar shell tightens on mobile.** `ProfileSidebarShell` and `ProfileEditSidebar` drop avatar size from `size-24` to `size-20` and padding from `px-6 py-7` to `px-5 py-6` below `sm`. Internal scroll is only active at `md+`.

The blueprint/agent card grids already used `grid-cols-1 sm:grid-cols-2 xl:grid-cols-3`, so no card-grid changes were needed.

## Migration

None required — pure CSS change.
