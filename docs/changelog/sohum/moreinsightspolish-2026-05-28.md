# More Insights polish

A multi-round design pass on the Insights page picking up from
`sohum/insightuipolish` (PR #1163). All changes are visual / behavioral
inside the Insights surface, with additive changes to shared primitives
(`Table`, `TableHead`, `UserBadge`). Migration: none.

## Reviewer checklist

### Page chrome

- [ ] Subtitle: *"Track usage, cost, and reliability across your agents."* → *"… across your organization."*
- [ ] `PageStarField` dropped; page now uses `PageContainer(outerClassName="bg-background")` like Agents / Explore / Blueprints. Fixes light-mode wash-out.
- [ ] Date label + time-range chips + scope switcher live on the **same row as the page title** (the prior sub-bar is gone). Date label hides on `< @md` widths so the row doesn't crowd.

### Stat cards + charts

- [ ] Centered *"No insights for this period / Go to Agents"* CTA card is gone. On an empty account the full layout still renders — stat cards at 0, both chart cards with their chrome, each showing *"No spend yet"* centered in the body (Monitor-tab pattern). Empty branch now also fires when the data array has entries but every value is zero (account has the date axis materialised but no actual spend).
- [ ] Left chart title: *Agent Spend* → **Agent spend over time**.
- [ ] Right chart title: *Total Spend* → **People spend over time**.
- [ ] Right chart users line: `var(--warning)` (orange) → `var(--primary)` (indigo). Yellow / red / green reserved for status states.
- [ ] Right chart spend line: `var(--muted-foreground)` → `var(--foreground)`. Stronger contrast vs the indigo users line in light mode.
- [ ] Right chart legend / tooltip label: *Users* → **By People**.

### Toolbar (above the table)

- [ ] `ViewToggle` redesigned: **People (N)** / **Agents (N)** pills with Lucide `Users` and `Bot` icons + tabular-num counts. Uses the canonical `ToggleGroup variant="word"` chrome — same sliding indicator as `KnowledgeBindingPicker` and the storybook reference.
- [ ] Chip-based multi-select filter retired. Single `FilterInput` bound to `?q=` filters agents by `agent_name` or people by `display_name / username / user_id` — whichever view is active.
- [ ] Placeholder: *"Search people or agents…"*.
- [ ] Switching views clears `?q=` so a People-view search term doesn't silently empty the Agents table (and vice versa).

### Top Spenders table

- [ ] **`<Table bare>`** — no rounded-card chrome, no "Spend" panel title. The table renders flat in the page surface; column-header row + per-row separators stay.
- [ ] Avatars column on agents view renamed **Users → People**; first column on users view renamed **User → Person**.
- [ ] Agents row renders the agent avatar (`BlueprintIdentity` `size=20`) inline before the agent name.
- [ ] People row's **Person** cell links to `/{username}` via new `linkToProfile` prop on `UserBadge`.
- [ ] **% Total** column on both views (`32.8%` format). Denominator is owned by the caller (`Insights.tsx` passes the un-filtered total) so percentages stay anchored while the user searches.
- [ ] All value cells render in `text-foreground` (was a mix with `text-muted-foreground`). Right alignment kept; mono dropped.
- [ ] Cap on **People** avatar chips: 3 visible (was 5) + `+N` overflow.
- [ ] **Overflow `+N` chips are now clickable popovers.** Both `UsersUsedAvatars` and `AgentsUsedChips` open a `bg-popover` card listing every entry — avatars + names; agent entries link to the agent page. Replaces the prior aria-only / hover-only treatment.
- [ ] Bucket rows render the lucide icon bare (muted-circle background dropped).
- [ ] Bucket rename: **Infrastructure → System spend**.
- [ ] Empty state copy: *"No user activity in this period"* → *"No activity from people in this period"*. Bucket count suffix: *"3 users"* → *"3 people"*.
- [ ] Unattributed/System-spend tooltip copy: *"Traces not associated with any user — typically background jobs, system tasks, or SDK calls…"*.
- [ ] Unidentified bucket tooltip copy: drops *"(Slack, Discord, etc.)"* → *"(Slack, etc.)"* (no Discord adapter yet). Tooltip is just the explainer text.
- [ ] Unidentified avatar tooltip + `+N` popover rows: label reads `Slack ID: <uid>` so the raw identifier is visible (no fabricated nickname).

### Table primitive (`@/components/ui/table`)

- [ ] `TableHead` gains three additive props: `sortable: boolean`, `sortDirection: "asc" | "desc"` (omit = inactive), `onSort: () => void`. Sortable head gets `cursor-pointer + hover` and forwards an `aria-sort` attribute. Only the active column renders an arrow (`↑`/`↓` in foreground); inactive sortable columns render no glyph.
- [ ] `Table` gains an optional `header` slot — renders inside the bordered container above the table for a unified panel chrome (no nested borders).
- [ ] `Table` gains a `bare` flag — suppresses the outer rounded-border chrome so the table flows in the page surface. Used by the Insights page.
- [ ] Storybook entries: `Sortable` + `WithPanelHeader` demonstrate the new props.

## Files touched

- `pages/Insights.tsx` — chrome consolidation, drop `PageStarField`, header row, `?q=` wiring, stable `% Total` denominators
- `components/activity/TopSpendersTable.tsx` — avatar + profile link, `% Total` (caller-owned), drop mono, bucket polish, sort wiring
- `components/activity/ViewToggle.tsx` — People / Agents pills with Lucide icons + counts; canonical word-toggle chrome
- `components/activity/CostOverTimeChart.tsx` — title + empty copy + all-zero empty branch
- `components/activity/ActiveUsersSpendChart.tsx` — title + empty copy + users line color + spend line contrast + legend rename
- `components/activity/UsersUsedAvatars.tsx` — cap at 3 visible + clickable popover for overflow
- `components/activity/AgentsUsedChips.tsx` — clickable popover for overflow
- `components/UserBadge.tsx` — `linkToProfile` prop
- `components/ui/table.tsx` — sortable props on `TableHead`; `header` slot + `bare` flag on `Table`
- `components/activity/user-classification.test.ts` — **new**, lifted from the deleted `UserFilterBar.test.tsx`
- `stories/Table.stories.tsx` — `Sortable` + `WithPanelHeader` stories

### Deleted

- `components/activity/MultiSelectFilterBar.tsx`
- `components/activity/AgentFilterBar.tsx`
- `components/activity/UserFilterBar.tsx`
- `components/activity/UserFilterBar.test.tsx` — `classifyUserId` cases moved to `user-classification.test.ts`; the bar-specific entries/popover cases retired with the component
