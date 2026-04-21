# Nav IA: global nav redesign, Explore page + Blueprints, mobile responsive header

## Summary

A full rework of the global navigation information architecture. The desktop header gains a compact scope switcher, centered nav links with count indicators, and updated org management placement. Community blueprint discovery moves to a dedicated Explore page. The Blueprints section simplifies to a single account-scoped view with new empty states for both Agents and Blueprints. The header is made properly responsive on mobile with a two-row layout, scrollable nav tabs, and container-query-based page content.

## Design

### Desktop header

**Scope switcher** — compact pill inline after the logo; username only (no display name), no border, `rounded-sm`. No default fill; `hover:bg-stone-200`. Org avatars resolved via `UserAvatar` (CDN handle lookup) instead of a generic building icon.

**Nav links** — centered using absolute positioning. Active state uses medium font weight. Agent and blueprint counts shown as `Tag` components next to each link; hidden when count is zero. Blueprints listed before Agents.

**Create Organization** — moved from the profile dropdown into the scope switcher dropdown as the last item, separated by a divider. Organizations list removed from the profile dropdown entirely.

**Explore** — promoted out of the nav tabs to an outline button with a globe icon in the header's right section. It's a global discovery feature, not account-scoped, so it doesn't belong in the tab row.

### Explore page

`/explore` is now its own top-level route with `bg-muted` background, showing all community blueprints. Previously nested under `/blueprints/discover`.

### Blueprints page

Collapses from a layout with sub-routes into a single page scoped to the active account. Blueprint grid expands to 4 columns at `lg`. Empty state uses a rich onboarding panel (mascots, gradient background) with "Create blueprint" and "Start from CLI" CTAs (links to `https://docs.astropods.com/install-cli`), plus a community blueprint grid below.

### Agents empty state

Simplified to a dashed container with a rocket icon, "No agents deployed yet" heading, and two CTAs: "Create blueprint" and "Explore from community". Dashboard stats are hidden when the list is empty.

**CTA copy** — standardized to "Create blueprint" throughout (was "New blueprint" / "Create agent" in some places).

### Mobile header

Below 1024px, the desktop header switches to a two-row layout:

- **Row 1** — logo, scope switcher (≥380px), Explore button, hamburger
- **Row 1.5** — scope switcher only, visible below 380px
- **Row 2** — horizontally scrollable nav tabs (Blueprints, Agents, Stores)

The breakpoint was raised to 1024px because the centered absolute-positioned desktop nav was colliding with the scope switcher and right-side actions at tablet widths.

At widths below 480px the labeled Explore button collapses to an icon-only `size="icon"` variant (globe icon, `aria-label="Explore"`). Above 480px the full labeled button is shown.

Feedback, Docs, and Blog move into the hamburger Sheet on mobile; on desktop they remain as text links in the right-side header. The hamburger content matches the desktop profile dropdown (Profile, Settings, Admin, Sign out, ThemeSwitcher).

Mobile nav tabs match the agent detail page style: teal active underline (`border-[var(--color-teal-600)]`), `text-heading-4`, `text-faint-foreground` for inactive state.

### Container queries on dashboard grids

`DashboardStats`, `DeployedAgentsSection`, and `DashboardToolbar` used viewport-based breakpoints (`sm:`, `md:`, `xl:`). These are replaced with container queries (`@[540px]`, `@[800px]`, `@[1100px]`) so layouts reflow based on their container width. The outer `flex-1` wrapper on `AgentDashboard` is marked `@container`.

### Background fill

The Layout root div now carries `bg-muted` directly, so there is no gap if `flex-1` on a page's content div doesn't fully expand. `min-h-dvh` is used instead of `min-h-screen` to account for dynamic browser chrome on mobile.

## Migration

No action required.
