# Monitor & Deployments Polish

## Summary

A focused polish pass on the Monitor and Deployments detail pages, improving visual consistency across controls, status indicators, and interactive elements.

## Design

- **Kebab menu icons** (breadcrumb header + past deploys table) updated to `text-foreground` for better visibility.
- **Traces "see more" button** replaced with the shared ghost `Button` component, now shows remaining count inline (e.g. "See 96 more") with a chevron that flips on expand/collapse.
- **Filter controls** (log time range, search input, traces "All statuses" multi-select, monitor time window) unified to use `var(--popover)` background. `SelectTrigger` component now defaults to `text-foreground`.
- **Deployments table status column** right-aligned to match other columns, and rendered with a colored dot + uppercase label using the same teal token as the breadcrumb header badge.
- **Ready indicator** checkmark moved to after the "ready" label.
- **Build ID badge** removed from the agent breadcrumb header; the update badge still appears when a newer build is available.

## Migration

No migration required.
