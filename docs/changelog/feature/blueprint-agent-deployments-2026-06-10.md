## Summary

Surface the agents a viewer has actually deployed from a blueprint, directly on the blueprint detail page. Before, there was no way to see your own deployments without leaving the blueprint and combing the agents list.

## Design

A new `SidebarDeployedAgents` section renders in the blueprint sidebar between the GitHub panel and the stats block. It pulls the viewer's deployments via the existing `useDeployments` query and intersects them with the blueprint's `build_id`s, so only deployments that originated from this blueprint surface.

Privacy is preserved by scoping the list to accounts the viewer belongs to. The component returns `null` for non-members and for blueprints with no matching deployments, so it never advertises the existence of deployments the viewer cannot see. Up to four rows show by default; the rest collapse behind a "Show N more" disclosure.

The shared `SidebarSection` info-badge affordance changed from a Radix `Tooltip` (hover-only) to a Radix `Popover` (click/tap). This makes the scope explanation reachable on touch devices, where the previous tooltip never opened. The popover hugs its content on desktop and wraps to fit on narrow viewports using `max-width: var(--radix-popover-content-available-width)` with `text-balance`.

## Migration

No action required. The new section is additive and only renders for account members with matching deployments.
