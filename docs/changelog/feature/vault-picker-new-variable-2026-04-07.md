# VaultPicker Inline Variable Creation

## Summary

Adds the ability to create a new variable directly from the VaultPicker dropdown, without navigating to settings. Also includes polish to the Organizations settings page.

## Design

**VaultPicker — New variable button:** A "New variable" button (with Plus icon) now appears in two places within the VaultPicker:

- In the empty state, below the description text
- In the non-empty state header row, right-aligned alongside "Select a reference"

Clicking either opens the existing `NewEntryDialog` modal. The popover closes first to avoid z-index conflicts. The modal shows "Saving to [account]" context so the user knows which account the variable will be created in. On success the mutation invalidates the variables query, so the picker refreshes automatically.

**NewEntryDialog — account context:** Added an optional `accountName` prop. When provided, a "Saving to [account]" line renders below the modal title to communicate where the variable will be saved.

**NewEntryDialog — form reset fix:** Form state (name, value, description, secret toggle) was not resetting when the dialog was closed programmatically after a successful save. Radix UI's `onOpenChange` doesn't fire on prop-driven closes, so state persisted into the next open. Fixed with a `useEffect` that resets all fields when `open` transitions to `false`.

**Organizations settings:** Replaced the colored initial avatar with a `BuildingOffice2Icon` on a stone background, matching the profile dropdown treatment. Updated subtext to "Manage your organizations and access settings".

## Migration

No migration required.
