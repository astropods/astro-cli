## Summary

The sort select on the dashboard toolbar had a `bg-background` fill, inconsistent with the search input which uses a white fill in light mode.

## Design

Updated `SelectTrigger` in `DashboardToolbar` to use `!bg-white dark:!bg-background`, matching the existing pattern on `FilterInput`.

## Migration

No action required.
