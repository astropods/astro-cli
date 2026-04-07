## Summary

"Request increase" buttons in the agent monitor dashboard and usage settings page did not show a pointer cursor on hover, making them appear non-interactive despite being clickable.

## Design

Added `cursor-pointer` to the two `<button>` elements rendering "Request increase": one in `ComputeUsageCard` (dashboard) and one in `UsageBar` (settings page). Browsers do not apply pointer cursor to `<button>` elements by default under some CSS resets, so this must be set explicitly.

## Migration

No action required.
