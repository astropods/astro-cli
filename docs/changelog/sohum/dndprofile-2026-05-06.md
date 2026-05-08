## Summary

Adds drag-to-reorder on the Blueprints tab of the account profile page. Owners can set a custom display order for their blueprints, persisted to the server via `PATCH /api/v1/accounts/:account`. Also includes several UI polish fixes: equal-height blueprint cards, compact filter dropdowns, and a unified tooltip provider for org avatars and the early adopter badge.

## Design

**Reorder mode state machine.** A `ReorderMode` type (`"idle" | "editing" | "saved"`) drives the tab. Clicking "Customize order" resets all filters (search, visibility, sort) and enters editing mode. Clicking "Save changes" commits the order, shows a brief "Saved ✓" confirmation, then returns to idle.

**Server persistence.** Blueprint order is stored in a new `blueprint_order text[] NOT NULL DEFAULT '{}'` column on the `account_profile` table. The existing `PATCH /api/v1/accounts/:account` endpoint accepts a `blueprint_order` field with the same PATCH semantics as other profile fields (nil = leave unchanged, non-nil = overwrite). `GET /api/v1/accounts/:account` returns the saved order in `blueprint_order`. The client seeds `bpCustomOrder` from `data.blueprint_order` on load and writes back via `useUpdateAccountProfile`.

**UX borrowed from `feature/profile-tabs`.** In editing mode, the whole card is the drag target (`cursor-grab`), not a separate button. The grip icon is `pointer-events-none` and positioned top-right — always visible in editing mode, revealed on hover in idle mode. A CSS wiggle animation (`card-draggable:hover`) reinforces drag affordance; a staggered entrance animation (`card-entering-edit`) plays when entering edit mode.

**Component split.** Drag logic lives entirely in `BlueprintsTab` — `localOrder` state is seeded from the `blueprints` prop when entering edit mode and updated on each drag. On save, `onSaveReorder(names)` propagates the final order to `IndividualProfile`, which fires the mutation and updates `bpCustomOrder`. No intermediate state is needed in the parent.

**Save error handling.** `handleSaveReorder` optimistically sets `bpCustomOrder` and transitions to "saved" mode, then fires the mutation with `onSuccess` (transition to idle after 1500 ms) and `onError` (revert `bpCustomOrder` to the previous value and return to "editing" mode). This prevents a false "Saved ✓" confirmation when the API call fails — the user is returned to the reorder UI and can retry.

**Server-side input cap.** `PATCH /api/v1/accounts/:account` now rejects `blueprint_order` arrays with more than 200 entries (HTTP 400), consistent with the existing 4-entry cap on `social_links`. Individual entry length is still capped at 255 characters.

**Custom order in idle mode.** When `bpCustomOrder` is set and sort is "newest", `visibleBlueprints` applies the saved order as a primary key, falling back to `published_at` for new blueprints not yet in the order array.

**CSS.** Three new global animations in `index.css`: `card-wiggle` (hover), `card-edit-entrance` (mode entrance), and the `card-draggable` / `card-entering-edit` utility classes that apply them.

**UI fixes.** Blueprint cards in the grid now have equal height — `h-full` added to `IdleBlueprintCard` and `SortableBlueprintCard` inner wrappers to complete the height chain from grid cell to card link. Filter dropdown items use `py-1 text-body-sm` to match the `size="sm"` trigger buttons. `TooltipProvider` is hoisted to wrap the entire `ProfileViewSidebar`, covering both the early adopter badge and org avatar tooltips from a single provider (org avatars previously used a native `title` attribute).

## Migration

**Schema migration required.** Run the following against the production database after deploying:

```sql
-- 1. Add the column (safe — default value backfills all existing rows as empty array)
ALTER TABLE account_profile
  ADD COLUMN IF NOT EXISTS blueprint_order text[] NOT NULL DEFAULT '{}';

-- 2. Backfill account_number for existing users, ordered by account creation date
WITH ranked AS (
  SELECT
    ap.account_id,
    ROW_NUMBER() OVER (ORDER BY a.created_at ASC) AS rn
  FROM account_profile ap
  JOIN accounts a ON a.id = ap.account_id
  WHERE ap.account_number IS NULL
)
UPDATE account_profile ap
SET account_number = ranked.rn
FROM ranked
WHERE ap.account_id = ranked.account_id;

-- 3. Reset the serial sequence so new signups continue from the right number
SELECT setval(
  pg_get_serial_sequence('account_profile', 'account_number'),
  (SELECT MAX(account_number) FROM account_profile)
);
```

Steps 2 and 3 only apply if `account_number` was recently added and existing rows have `NULL`. New signups after the backfill will auto-increment correctly from the updated sequence. No user-facing action required.
