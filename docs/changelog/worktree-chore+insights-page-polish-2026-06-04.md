## Summary

Polish pass on the Insights page covering visual consistency with the rest of the dashboard, a shared toggle primitive, treatment of unidentified users, and removal of the legacy tombstone path for deleted agents.

## Design

**Shared PillToggle primitive.** The time-range selector and the view toggle (Agents / People) previously had their own bespoke implementations. Both now use a single `PillToggle` component with `sm` / `md` variants. The active indicator is a Framer `layoutId` spring that animates between options, and the track / active-pill colors are recalibrated against the new chart-card surface so the two toggles read as the same control in different sizes.

**Table chrome aligned with chart cards.** The deployments table container now uses the same `bg-card` / `dark:bg-surface` surface as the chart cards above it. Header row lightened to `bg-muted/40` in light mode. Column headers are `align-bottom` on sub-md and `align-middle` on md+ so wrapping headers don't drag single-line headers to vertical-center.

**Per-row unidentified users.** The "Unidentified · N people" aggregate bucket is gone. Each unknown `user_id` now renders as its own row with a soft-circle avatar plus the mono ID. The same soft-circle treatment is mirrored in `UsersUsedAvatars` so the People view and the Used-by column are visually consistent. (A row-level "Invite to astro" CTA is deferred to a follow-up PR — it needs a Slack-DM invite flow rather than reusing the email-based dialog.)

**Removed deleted-agents path.** The "Show deleted agents" checkbox, the `?archived=true` URL state, the `includeArchived` plumbing through `useInsightsData`, `useActiveSpendSeries`, `useAccountActivitySummary`, `useDeploymentsSummary`, and the observability query-key factories, and the tombstone row rendering (`isDeleted`, `MotionTableRow`, `AnimatePresence`) are all removed. Archived rows are no longer fetched.

**Typography normalization.** Name column now uses the `Table` primitive defaults — `UserBadge`'s `text-body-sm` override and the `font-medium` on agent names are dropped so the column matches every other table on the page. Unidentified `user_id` text dialed down to 13px so the mono ID sits next to Geist body text without looking oversized. Used-by and Agents-Used chip avatars bumped to 24×24; agent avatars use `rounded-[3px]` to match `BlueprintCard`.

**Shared "Show more" affordance.** The collapse/expand pattern that lives on the Monitor traces table is lifted into the shared `Table` primitive as `<TableShowMore hiddenCount expanded onToggle />`, a sibling `<AnimatedRow>` for the reveal animation, and a new `footer` slot on `<Table>` (mirroring the existing `header` slot). Both Insights views (Agents + People) now collapse long lists to the top 5 by the active sort key with a "Show N more" toggle; clicking it reveals everything past row 5 with a small staggered fade-in. `TracesTable` swaps its inline button for the shared component — pixel-identical to before. Default visible row count is 5 for Insights and 10 for Monitor traces (different page layouts; Insights stacks stat cards + charts + table above the fold). A new `Rank` column shows the row's position in the active sort, ranked across real + unidentified users by cost; System spend stays pinned at the bottom.

**Other touches.** Slack identity row wraps icon + label in a single `<a>` with `hover:underline` on the link itself, matching the member/agent row link pattern. Panel header reordered: search restored to a fixed width with placeholder "Search by name".

## Migration

No action required.
