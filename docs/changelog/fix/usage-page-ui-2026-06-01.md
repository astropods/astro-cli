## Summary

The Usage settings page had a flat, unsorted grid of meters with inline status styling and a static info box for compute units. This made it hard to scan at a glance, especially for accounts with many meters across different resource types.

## Design

Meters are now grouped into three categories — **Agents**, **Knowledge**, and **Account** — each rendered as a labelled section with its own 2-column grid. Categories with no active meters are hidden entirely.

The per-card layout was also reworked:
- The "Request increase" link moves to the card header (right-aligned), scoped to meters that have a quota
- Compute Unit explanation moves from a static info box at the bottom of the page into an inline tooltip on the unit label, keeping it close to the data without taking up persistent space
- Status badges in the quota increase requests table now use `StatusBadge` instead of hand-rolled inline styles
- The `"Full"` state (usage ≥ 100%) is surfaced as a `Tag` chip next to the usage number

Separately, the Account settings subtitle copy was updated to be more descriptive, and the mobile nav dropdown in `sidebar-layout` was switched from a hardcoded `bg-slate-200` to the semantic `bg-secondary` token so it themes correctly in dark mode.

## Migration

No action required.
