# Insights UI polish

## Summary

A batch of Insights page UI work, extracted from `sohum/usertogglepolish`
so that one stays focused on the client-side slicing architecture. This
PR is the source of truth — supersedes PR #1124 (per-agent Users column)
and bundles the visual + interaction polish from a long-running iteration
session.

The biggest user-visible additions:

- **Two charts side-by-side**: Agent Spend (per-agent breakdown) on the
  left, Total Spend (dual-line: active users + total $) on the right.
- **Users column** on the agents-view table with avatar chips.
- **Toggle independence**: the date-range chip drives the charts only;
  the agent / user view toggle drives the table only. Flipping the view
  no longer reloads or remounts the upper page.

## Design

### Layout

- **Top bar**: date label (left) ⟷ time-range chips (right), inline.
- **Stat cards**: SPEND / REQUESTS / TOKENS, no sparklines, change %
  rendered inline next to the value (not below) so the card height is
  constant across every range — including "all time."
- **Charts**: 2-column grid, both always visible, both react only to the
  date-range chip.
- **Above the table**: by-agent / by-user toggle on the left, search bar
  on the right. Toggle controls the table only.
- **Table**: bare `<Table>` primitive (matches `AuditLogView`'s
  treatment), no `<Card>` wrapper, no "Top Spenders" heading.

### Charts

- **Agent Spend** (left) — existing `CostOverTimeChart`. Bar variant for
  bounded ranges, line for all-time. Per-agent stack.
- **Total Spend** (right) — new `ActiveUsersSpendChart`. Recharts
  dual-axis line chart: total spend $ on the **left** Y-axis (grey),
  active user count on the **right** Y-axis (orange). X-axis enumerates
  every day in the range, zero-filling inactive days. Each line has
  inline color-matched dots at every point + a larger `activeDot` on
  hover. Tooltip + legend below.
- Both charts use matched left-axis width (56px) and right reservation
  (52px) so their plot areas are pixel-aligned across the row.
- Data flow: a new `useActiveSpendSeries(account, range)` hook fetches
  the all-time `groupBy=user` summary once and slices client-side by
  the URL range, mirroring PR #1149's slicing approach — every range
  toggle is a 0-round-trip recompute.

### Users column on the agents-view Top Spenders table

- New `users_used` field on `BlueprintSummaryEntry` (server-side).
  Populated by a single account-level Q_tags Langfuse query running in
  parallel with the existing per-deployment fan-out — wall time
  unchanged, Langfuse cost vs client-side inversion identical.
  Fail-open: a Q_tags failure logs a warning and surfaces empty
  `users_used` arrays rather than 502-ing the headline blueprints view.
- Same tag scope as the existing fan-out
  (`tags any-of visibleTagValues`), so `users_used` automatically
  scopes to currently-visible deployments. Multi-region deployments of
  the same `agent_name` dedupe naturally.
- Frontend: new `UsersUsedAvatars` component mirrors `AgentsUsedChips`
  — up to 5 avatars + "+N" overflow chip, resolves member info via
  `useAccountMembers`. Empty renders as `—`.

Supersedes PR #1124, which only rendered a count.

### Stat-card change-badge: smooth transitions

- Badge now renders inline with the value on the same baseline so the
  card row height is pinned by `text-heading-2`, never by the badge's
  presence. Result: card height stays identical across 7d / 14d / 30d /
  all-time.
- The badge wrapper is always mounted when `hasChangeApi`; visibility
  is controlled by an `opacity-0 ↔ opacity-100` swap with
  `transition-opacity duration-200`. Toggling between ranges fades the
  badge in / out over 200ms instead of remounting the node.
- Sparklines dropped from the stat cards entirely — too flashy.
- The `— —` placeholder is gone. Cards opt out of `TrendIndicator`
  (`showTrend={false}`); zero change (`value === 0`) no longer renders
  an em-dash arrow.

### Toggle behavior

- **Date-range chip** → charts only. Table data is fetched all-time and
  filtered client-side — date-range never re-fetches the table.
- **View toggle** (By Agent / By User) → table only. Both views share
  the same chart data sources, so the chart subtree never re-renders.
- `shouldRevalidate` no longer fires on `?view=` changes; the toggle is
  a pure client-side state flip.
- `AgentsTab` / `UsersTab` are merged into a single `InsightsView`
  component. Hooks for both views' table data run unconditionally
  (cached after first load); only the filter bar + table swap inside
  `InsightsBody` on toggle. Top of the page never remounts.

### Filter bar

- Placeholder is now `Search agents...` / `Search users...` to signal
  the input is typable (the typing always worked, but the old "{N}
  active agents" placeholder hid that).
- The "All agents" / "All users" shortcut in the dropdown only renders
  when the search input is empty — once you start typing you're
  narrowing, not selecting all.

### Column-header counts

- `TopSpendersTable` gains a `totalCount` prop; the `Agent` / `User`
  header cells render the entity count inline in faint text (e.g.
  `Agent 12`, `User 47`) — matches the tab-counter pattern used on the
  Blueprints / Agents / Hearts header strip.

### ViewToggle background

- The non-active toggle item used to let the `PageStarField` bleed
  through (`var(--input-background)` is translucent over the star
  field). Switched to `bg-card` so the pill chrome is opaque against
  any page chrome.

### Misc copy

- "Spend" column header → "Total Spend" (matches chart heading).
- "Spend over time" chart heading → "Agent Spend" (left chart).
- "Active Users + Spend" chart heading → "Total Spend" (right chart).
- "Unauthorized" user bucket → "Unidentified" across constants,
  sentinel value (`__unidentified__`), color export, chip label, and
  tooltip wording. HTTP 401 "Unauthorized" references elsewhere are
  unrelated and left alone.
- Agent rows in the table: dropped the inline `top_model` text and the
  `BlueprintIdentity` avatar — name-only link.

### Background data refresh

`ACTIVITY_QUERY_OPTS` (shared by the activity / blueprints / users
summary hooks) already had `staleTime: 5min` and
`refetchOnWindowFocus: false`. Gap: a user who opens Insights and
stares at the dashboard for 30 minutes won't see fresh data — the
stale flag only triggers a refetch on the next interaction, and focus
refresh is intentionally off. Adds `refetchInterval: 5min` so
background polling closes that gap, aligned with the existing
staleTime.

## Migration

None. Server changes are strictly additive (`users_used` field on
`BlueprintSummaryEntry`); existing clients ignore it.
