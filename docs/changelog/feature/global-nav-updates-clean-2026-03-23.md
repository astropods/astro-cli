## Summary

Moves Docs and Blog nav links from the left side of the global header to the right, separating external links from internal app navigation.

## Design

`publicNav` is split into `publicNav` (internal routes: Blueprints, My Agents) and `externalNav` (Docs, Blog). On desktop, `publicNav` renders on the left alongside the logo while `externalNav` renders on the right before the auth controls. On mobile, both arrays are merged for the sheet menu.

Left nav active state is updated from `font-semibold text-primary` to `text-foreground`, with `text-muted-foreground` for inactive items.

## Migration

No changes required.
