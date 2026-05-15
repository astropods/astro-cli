## Summary

Several visual polish issues on the blueprint detail page's GitHub and Repository sidebar panels: incorrect icon colors in dark mode, inconsistent spacing, and a mislabeled stat.

## Design

**Dark mode icon fix — `SidebarRepository`:** The GitHub (and GitLab/Bitbucket) icon was always loading the `"light"` variant via `getIntegrationIconUrl`, making it invisible in dark mode. Now renders two `<img>` elements — one per variant — toggled with `dark:hidden` / `dark:block`.

**GitHub panel icon consistency:** The lucide `Github` icon in `GitHubConnectionPanel` has been replaced with the same themed `<img>` pattern used in `SidebarRepository`, so both panels use the same icon source and dark mode behavior.

**`SidebarRepository` polish:** Added an `ExternalLink` arrow after the repo label (matching the GitHub panel's link row). Tightened gap from `gap-2.5` to `gap-1.5` to match. Removed `hover:decoration-primary` so the underline inherits `currentColor` (white in dark mode) instead of indigo.

**GitHub panel dropdown:** Moved the `<DropdownMenuSeparator />` to sit above Disconnect rather than above Rebuild branch. Added `text-destructive` to the `Link2Off` icon so it matches the red label.

**Stat label rename:** "Deployments" → "Deploys" in `SidebarStats`.

## Migration

No action required.
