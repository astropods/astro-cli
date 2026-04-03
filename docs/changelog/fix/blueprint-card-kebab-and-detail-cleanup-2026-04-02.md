## Summary

Blueprint cards were missing the owner actions menu on the Blueprints page, while the detail page incorrectly showed both the kebab menu and a pencil icon overlay on the avatar. This aligns ownership-gated UI to appear consistently on cards wherever they are rendered.

## Design

The `BlueprintCard` already supported an `onArchive` prop to conditionally render the three-dot menu. The fix has two parts:

**Detail page cleanup** — Removed `onArchive` from `BlueprintDetailHeader` and `BlueprintDetailContent` entirely. The avatar edit interaction is preserved using the existing hover-to-camera overlay pattern (matching Account Settings) rather than the persistent pencil icon.

**Per-card ownership on list views** — `BlueprintListView` now accepts an `ownerAccounts?: Set<string>` prop. Each card receives `onArchive` only when `blueprint.account` is in the set. `AccountBlueprintsList` constructs the set from the account it already has. The Discover page derives the set from `useAuth().accounts`, so cards owned by the logged-in user show the menu while others don't.

## Migration

No changes required.
