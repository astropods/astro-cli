# Nav IA: in-page scope, shared page shell, Explore page, mobile header

## Summary

Reworks the global navigation so scope (personal vs. organization) is
chosen per page rather than from the header. Extracts a shared page
shell used by Blueprints, Agents, and Explore so all three dashboards
share padding, max-width, and title/description structure. Routes
`/explore` as a dedicated top-level page, collapses the old
`/blueprints/*` sub-routing into a single account-scoped Blueprints
page, and makes the header responsive on mobile.

## Design

### Header

**Desktop** — logo plus the primary nav (`Blueprints`, `Agents`, and
`Stores` when the `knowledgeStore` experiment is on) sits on the left;
external links, Feedback, Explore, and the profile dropdown sit on the
right. No scope switcher and no per-tab count badges.

**Explore button** — outline button in the header's right section,
using Lucide's `Telescope` icon (`strokeWidth={1.5}`). Replaces the
earlier globe treatment in the header and in the Agents empty state.

**Mobile** — two-row layout below 1024px:

- **Row 1** — logo, Explore (icon-only <480px, labelled above), and
  the hamburger. Profile, Settings, Admin, Sign out, Feedback, Docs,
  and Blog live inside the Sheet drawer.
- **Row 2** — horizontally scrollable nav tabs styled to match the
  agent detail tabs (teal active underline).

### In-page scope switcher

Each page that is account-scoped (Blueprints, Agents) renders a
`PageScopeSwitcher` next to its `<h1>`. The switcher is a thin wrapper
around `OrgSwitcher`, which keeps a single pill-style trigger (avatar +
handle + chevron, `aria-label="Switch account"`). Dropdown items show
each account's handle with a star to pin a non-personal default; the
last item is a `Create organization` link.

State is shared across pages through `ActiveAccountProvider` in
`Layout.tsx` (localStorage-backed via `astro:default-account`), so
choosing an org on Blueprints persists on Agents and any other
consumer of `useActiveAccount`.

### Shared page shell

Two primitives in `components/PageLayout.tsx`:

- `PageContainer` — full-bleed `bg-muted` wrapper with an inner
  `@container max-w-[1500px] mx-auto` content column and unified
  responsive padding (`px-6 pb-6 pt-6 md:px-8 md:pb-8 md:pt-8`).
  Accepts an optional `style` passthrough for page-level background
  treatments (used for the Agents empty-state radial gradient).
- `PageHeader` — title + optional description, plus slots for an
  `adornment` (rendered inline next to the title, e.g. the scope
  switcher) and an `action` (rendered on the right, e.g. a primary
  CTA).

Blueprints, Agents, and Explore all render through these, so their
padding, max-width, title sizing, and description styling match.

### Routes

- `/explore` — dedicated top-level route using `PageContainer` +
  `PageHeader`. Previously nested under `/blueprints/discover`.
- `/blueprints` — single page showing the active account's blueprints;
  the sub-routes `/discover`, `/personal`, and `/:account` are gone,
  along with the orphaned `BlueprintsSidebar` and the
  `blueprintsPaths` helper. Empty state is an onboarding panel
  (mascots, two CTAs) followed by a community blueprint grid.
- The blueprint detail breadcrumb now links the account crumb to the
  account profile (`/:account`) instead of the removed
  `/blueprints/:account` route.

### Blueprint detail breadcrumb

On widths below `sm`, the breadcrumb collapses to a single item
showing the author's avatar and handle (linking to their profile).
Desktop keeps the full `Blueprints › account › name` chain. The bar
uses `min-h` + `flex-wrap` + `break-all` so long names wrap rather
than overflow.

### Agents and Blueprints

- **Agents empty state** — dashed panel with a rocket icon, a
  `Create blueprint` CTA, and an `Explore community blueprints` CTA
  (Telescope icon). Dashboard stats hide when the list is empty; the
  page picks up a top-anchored radial gradient in that state.
- **Blueprints empty state** — onboarding panel with `AgentMascots`,
  `Create blueprint` and `Start from CLI` CTAs, followed by a
  "Explore community blueprints" grid linking out to `/explore`.
- **Dashboard grids** now use container queries (`@[540px]`,
  `@[800px]`, `@[1100px]`, `@[1200px]`) so `DashboardStats`,
  `DeployedAgentsSection`, `DashboardToolbar`, and `BlueprintListView`
  reflow based on their container width.

### Settings

`SettingsLayout` and `OrgSettingsLayout` split the `bg-surface` fill
out to a full-width wrapper so the lighter surface no longer gets
clipped to the 1120px content column (leaving the parent `bg-muted`
showing on the sides).

### Layout root

`Layout.tsx` wrapper is `min-h-dvh` (accounting for mobile browser
chrome) with `bg-muted` applied directly so there's no visible gap if
a page's `flex-1` content doesn't fully expand.

## Migration

No action required.
