## Summary

Creating a new variable from a variable field's vault picker (on the deploy form and the settings → variables page) opened the New variable dialog empty, forcing the user to retype the key and value they had already entered in the field. While fixing this, two issues with the same dialog surfaced: its desktop width never matched the value the markup requested, and its row layout was unusable on mobile.

## Design

**Prefill.** The value flows from the field that launches the picker down to the dialog. `VariableField` passes the field's variable name (`newVarName={fieldKey}`) and its typed value (`newVarValue`, blank when the field already holds a vault reference) to `VaultPicker`, which forwards them to `NewEntryDialog` as `initialName` / `initialValue`. On open, the dialog seeds the first row's key (normalizing whitespace to `_`, matching the key input's own behavior) and value, and moves autofocus to the value field whenever anything is prefilled.

**Dialog width.** The dialog requested `max-w-[720px]`, but the base `DialogContent` ships `sm:max-w-lg`. tailwind-merge treats unmodified and `sm:`-modified utilities as separate groups, so both survived and the responsive default silently won at `sm`+ widths — capping the dialog at 512px regardless of the requested 720px. Changing to `sm:max-w-[720px]` overrides the default at the same breakpoint so the intended width actually applies.

**Mobile layout.** The Key / Value / secret-toggle row was a fixed three-column grid that crammed on small screens. It is now `grid-cols-1 sm:grid-cols-[…]` with the explicit `col-start` / `row-start` placements scoped to `sm:`, so fields stack vertically on mobile and return to three columns at `sm`+. The "Saving to &lt;account&gt;" line is left-aligned (it was inheriting `DialogHeader`'s `text-center` default on mobile).

**Shared surface.** `NewEntryDialog` backs both the deploy-form vault picker and the settings → variables modal, so all of the above applies to both entry points from a single change.

## Migration

No action required.
