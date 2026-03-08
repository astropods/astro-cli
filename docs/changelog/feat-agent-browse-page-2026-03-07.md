# Redesign browse page layout with SidebarLayout primitives

## Summary

The browse page (Hire) was restructured to use a new set of lightweight `SidebarLayout` primitives (`SidebarLayout`, `SidebarNav`, `SidebarNavItem`, `SidebarBody`) that provide a responsive sidebar+body shell. This replaces inline layout markup and extracts reusable components for any future sidebar-nav pages. The unused 727-line shadcn `sidebar.tsx` scaffold was removed.

## Design

- **`sidebar-layout.tsx`** — Four composable primitives: `SidebarLayout` (flex shell), `SidebarNav` (responsive nav container with optional label), `SidebarNavItem` (button with active state and `aria-current`), and `SidebarBody` (main content area). Horizontal on mobile, vertical sidebar on desktop.
- **`CategorySidebar`** — Refactored to consume `SidebarNav`/`SidebarNavItem` instead of owning its own markup.
- **`Hire` page** — Categories changed from dynamic (computed from agent tags) to a static list. Heading now reflects the selected category. Layout uses the new primitives.
- **Theme** — `ink` semantic token lightened for better readability on the warm `surface` background.

## Migration

No migration required.
