## Summary

Updates copy and visual polish on the `AgentDetailSidebar` component on the agent detail page.

## Design

- Renamed the CTA button label from "Hire this agent" to "Install this agent" to better reflect the action.
- Matched the installs icon (`ArrowDownTrayIcon`) color to `text-foreground` so it aligns with the adjacent text.
- Reduced sidebar section body padding from `py-3.5` to `py-3` for tighter, more consistent spacing across all section cards.
- Normalized the Details section specifically: body wrapper set to `py-1`, rows set to `py-2`, giving consistent top/bottom padding that matches the other containers.
- Updated `SidebarSection` to use `cn()` for class merging so `bodyClassName` overrides properly — previously `py-*` overrides were silently ignored due to Tailwind CSS ordering.

## Migration

No changes required.
