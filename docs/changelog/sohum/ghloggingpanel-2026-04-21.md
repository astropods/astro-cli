# Build Log Viewer

## Summary

The GitHub connection panel had no useful log visibility — clicking a build row showed raw text with no structure. This adds a `BuildLogViewer` component that surfaces build pipeline logs in a navigable UI inside the existing build logs dialog.

## Design

**`BuildLogViewer`** is a self-contained component that takes a list of components (agent, ingestion-startup, etc.), each with raw log text, status, and duration.

- **Tabs** — when there are multiple components, they appear as tabs (agent | ingestion-startup | …). With a single component, a static label row replaces the tab bar so the container name is still visible.
- **Section accordion** — within each tab, the raw log string is split on `=== section ===` delimiters. `events` and `ecr-login` sections are filtered out; `buildkit` is renamed to `build`. Each section is expandable with a line count badge.
- **Log line styling** — each line is parsed for an optional ISO timestamp and log level (INFO/WARN/ERROR/…), then rendered with monospace layout and per-level color using shared `log-utils` utilities.
- **Header** — shows "Build Logs" title with a `{GitHub icon} commitSha → {Astro icon} buildId` breadcrumb. If `buildId` is not yet assigned (build still queuing), the Astro slot shows a spinner + "pending". Total duration appears right-aligned when available.
- **Durations** — per-component duration (from `build.components[].started_at/completed_at`) trails the tab label; total duration (from `build.enqueued_at`) trails in the header.
- **Status icons** — checkmark (succeeded), X (failed), spinner (building), clock (pending) replace text status labels.

**`AstroIcon`** — a new hollow inline SVG component using `stroke="currentColor"` so it scales and recolors like a Lucide icon. Matches the GitHub icon style in the log header breadcrumb.

**Storybook** — `BuildLogViewer.stories.tsx` and `AstroIcon.stories.tsx` cover all states: loading, error, single-component (no tabs), multi-component mixed, building (pending build ID), failed, and full dialog context.

**Storybook global decorator** — pre-existing `useAuth` and `MemoryRouter` errors in Storybook were fixed by adding `AuthContext.Provider` with a mock context and `MemoryRouter` to the global decorator in `preview.ts`, so all stories work without per-story boilerplate.

**RepoPicker polish** (included from base branch):
- Debounce reduced 300→120ms for snappier search
- Dropdown uses fixed height (`h-48`) instead of `max-h-52` to prevent choppy resize on filter
- `connections` prop made optional to fix CI typecheck

## Migration

No changes required.
