## Summary

On the blueprint detail page, the breadcrumb showed `example / mybp` as a single unclickable label. Clicking on the scope (account) portion had no effect, making it impossible to navigate back to all blueprints under that account from the breadcrumb.

## Design

The breadcrumb now renders three separate items — `Blueprints`, the account name (linked to `/blueprints/:account`), and the blueprint name (current page, no link) — using the existing `blueprintsPaths.account()` route helper. The `PageBreadcrumb` component already supports linked items via the `to` prop, so no component changes were needed.

## Migration

No action required.
