## Summary

Polish pass on the account profile page — tightens the visual shell, removes unnecessary UI controls, and improves layout responsiveness.

## Design

**App shell background**: `Layout` and `index.css` now use `bg-background` instead of `bg-muted`/`bg-surface`, giving the profile page (and all routes) a cleaner base layer. The `GradientGridWash` dark-mode grid opacity was also increased from `0.2` to `0.6` for better visual presence.

**Single-tab heading**: `ProfileLayout` now counts visible tabs (`blueprints + agents? + hearts?`). When only one tab is visible (the common case for public profiles without deployed agents or hearts), the tab bar is replaced with a plain `<h2>` heading. This avoids the awkward UI of a single underlined tab with no peers.

**Tab bar spacing**: Removed the `pt-4` top padding from the multi-tab container on `md:` and up so tabs align flush with the sidebar. Mobile retains `pt-4` for breathing room. Removed the `-mb-px` offset from `TabButton` and added `bg-background` to the sidebar Edit profile button to fix a surface stacking issue.

**Container queries for grids**: All three tab grids (Blueprints, Agents, Hearts) switched from viewport breakpoints (`sm:grid-cols-2 xl:grid-cols-3`) to container queries (`@[540px]:grid-cols-2 @[900px]:grid-cols-3`). The `<main>` element is now a `@container`, so the grid reflow responds to the available content width rather than the full viewport — relevant when the profile sidebar is open or the panel is resized.

**Simplified tab toolbars**: Search and sort toolbars in each tab are now hidden when the list is empty and no filter is active. Previously the toolbar always rendered even on an empty profile. The `BlueprintsTab` visitor empty state was also simplified to plain text (no card chrome) to match the lighter treatment used by AgentsTab and HeartsTab.

**HeartsTab sort removed**: The sort dropdown (`Most hearts / Name A–Z`) was removed from HeartsTab. The sort state and related props were removed from `AccountProfile`, `ProfileLayout`, `HeartsTab`, and the test file.

## Migration

No action required.
