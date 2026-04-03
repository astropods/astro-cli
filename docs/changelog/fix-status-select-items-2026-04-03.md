# Fix Status Filter Select Items

## Summary

The status filter dropdown in the dashboard toolbar was rendering colored dots next to each status option. These color indicators were removed and the options are now sorted alphabetically for consistency with other filter dropdowns.

## Design

`STATUS_COLORS` and the per-option `color` prop have been removed from `DashboardToolbar`. `STATUS_OPTIONS` is now derived directly from `deploymentStatusLabel` and sorted alphabetically by label. `MultiSelectItem` no longer receives a `color` prop.

## Migration

No action required.
