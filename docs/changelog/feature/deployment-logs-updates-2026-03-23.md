# Deployment Detail UI Polish

## Summary

Small polish pass on the deployed agent detail view — standardizing buttons, icons, and badges to use the design system rather than one-off inline styles.

## Design

**Configure button** (`ActiveDetailView`, `ConfigurePanel`) — replaced a custom `<button>` with inline styles with the shared `Button` component (`variant="outline"`, `size="default"`). Active/toggled state uses the existing `data-active` support on the outline variant. Icon updated from Lucide `Settings2` to Heroicons `Cog6ToothIcon`.

**Pause / Resume buttons** (`ActiveDetailView`) — same treatment as Configure. Swapped custom `<button>` elements for the `Button` component. Icons updated to Heroicons `PauseCircleIcon` / `PlayCircleIcon`.

**Build ID badge** (`ActiveDetailView`) — removed the build ID `InlineBadge` from the agent name header. The "new build available" badge is retained since it's actionable.

**Services count badge** (`DeploymentsTab`) — replaced a raw `<span>` with `InlineBadge`. Added `variant="fill"` and `shape="square"` props to `InlineBadge` to support this pattern reusably. Label font updated to `var(--text-mono-sm)`.

**Traces empty state** (`MonitorTab`) — subtext switched from Geist Mono to Geist Sans (`var(--font-sans)`) with `var(--text-body)` size. Copy updated to "Trace data will appear here after the first request".

## Migration

No changes required.
