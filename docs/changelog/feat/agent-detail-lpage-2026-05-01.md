# Agent Detail Page Redesign

## Summary

Replaces the existing agent detail page with a redesigned, tab-based layout that consolidates deployment, monitoring, and configuration views into a single cohesive experience. The new page uses an immersive header with a canvas star field, squircle avatar, and agent identity bar, with content organized into Deployments, Monitor, and Configure tabs.

## Design

The page is composed of three layers:

- **Shell** (`AgentDetail`): Owns the agent query, star field background, identity header (name, status toggle, kebab menu), and the tab bar. Renders the active tab as a child route.
- **Tab pages** (`AgentDeployments`, `AgentMonitor`, `AgentConfigure`): Each tab is a standalone route component that fetches its own data via TanStack Query hooks and renders into the shell's content area. A shared `SidePanel` pattern handles right-side slide-in panels (trace detail, configure, etc.).
- **Shared components** (`agent-detail/`): Reusable primitives — `Squircle` avatar, `PodGraph`/`PodTile` for pod visualization, chart components (`TokenUsageChart`, `RequestVolumeChart`, `LatencyCard`), `TracesTable`/`TraceDetailPanel` for observability, and `DeploymentTile`/`DeploymentHistoryPanel` for deployment history.

Key decisions:
- Chart colors are derived from the agent's accent color via `deriveChartColors()`, producing dark/light mode palettes from a single hex input.
- Pod layout uses a force-directed graph (`PodGraph`) with tile measurements for responsive pod visualization.
- The star field is a canvas animation with pluggable direction strategies and reduced-motion support.
- Light/dark mode is fully supported across all components using semantic theme tokens.

## Migration

No migration required. The old detail page files have been removed and routes updated.
