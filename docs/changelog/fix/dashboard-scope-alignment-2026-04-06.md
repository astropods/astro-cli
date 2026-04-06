# Fix: Dashboard scope selector alignment

## Summary

The "View" scope selector (`OrgSwitcher`) was placed in a separate flex column from the dashboard greeting, causing it to render below the stats row at larger screen sizes. This made it visually disconnected from the heading it controls.

## Design

The hero row now uses a single `flex` container with `justify-between` so the h1 and scope selector always share the same line. Below `sm` (640px) the row switches to `flex-col-reverse`, stacking the selector **above** the heading rather than below it. The "View" label is also hidden below `sm` since the dropdown value already shows the active account name.

## Migration

No changes required.
