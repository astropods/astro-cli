## Summary

Several components on the deployment detail page had oversized border radii after the DeploymentsTab decompose in PR #518. The service accordions used `rounded-lg` (~19px) and filter buttons/search used `rounded-md` (12px), both visually inconsistent with the 8px buttons beside them. Additionally, these components used Lucide icons instead of the project-standard Heroicons.

## Design

- **Service accordions** — `rounded-lg` → `rounded-sm` (6px) to match the compact, data-dense style of the deployment panel.
- **Filter buttons, search input, dropdown** — `rounded-md` → `calc(var(--radius-sm) + 2px)` (8px) to match the Button primitive's border radius.
- **Icon consistency** — Replaced Lucide `Search`, `RefreshCw`, `Copy`, `Check` with Heroicons `MagnifyingGlassIcon`, `ArrowPathIcon`, `Square2StackIcon`, `CheckIcon`.
- **CopyButton shared component** — Upgraded from a raw `<button>` to `<Button variant="outline" size="icon">` so it picks up the design system's hover state and border radius automatically. Icon size bumped from 12px to 16px to match the icon button standard.

## Migration

None required.
