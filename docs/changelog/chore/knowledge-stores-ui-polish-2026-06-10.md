# Knowledge stores UI polish

## Summary

Visual and structural polish across the Knowledge Stores surfaces (list, detail, new-store flow), plus a dark-mode icon fix that affected provider icons. No API or data model changes; focus is on hierarchy, dark-mode parity, responsive behavior, and consistency with the rest of the app's design language.

## Design

**Detail page header**
- Replaced the sticky `PageBreadcrumb` with an inline monospace breadcrumb integrated into the page content. Removes a layout layer and matches breadcrumb treatment used elsewhere.
- Overview tab icon switched to lucide `BookOpen`, aligning with the lucide-first direction in newer surfaces.
- Right-aligned metadata: a single chip combines provider + mode (e.g. "PostgreSQL · External") with the provider icon inline. The ARN/URL tag uses a label/divider/value/copy composition that truncates the value at smaller widths instead of wrapping (`min-w-0 truncate` on the value, `shrink-0` on the label and divider).
- Dropped redundant "bound agents" and "created date" chips from the header — both surface elsewhere (Agent bindings section, knowledge table) without duplication.

**Cards (Overview tab)**
- Metric grid stacks single-column on mobile (`grid-cols-1 sm:grid-cols-2 lg:grid-cols-4`).
- Metric cards, Agent bindings, Event log, and the new-store provider list all use `bg-muted/30`, the codebase's existing recessed-fill pattern (KnowledgeBindingPicker, LinkConfirmDialog, BlueprintDetailSidebar). Removes the prior mismatch between `bg-card` and `dark:bg-surface` that read as two different elevation levels in dark mode.

**Settings tab**
- Centered, narrow layout (`max-w-2xl mx-auto`) using the existing `FormSection` component. `FormSection.description` widened to `ReactNode` so the Configuration section can embed a "Contact us" link inline.
- `FormSection` divider lightened from `border-border-strong` to `border-border` to match the lighter dividers used on `/account`.
- **Credentials card** uses a divided-row table layout (label / masked value / per-row eye + copy) instead of stacked inputs. Per-row reveal toggle and copy button both use lucide icons (`Eye`/`EyeOff`, `Copy`/`Check`) for stroke-weight consistency. Loading state mirrors the row structure so the layout doesn't jump on fetch.
- **Hidden state** renders a blurred ghost of the credentials table with a `Reveal credentials` button overlaid in the center. Clicking it triggers the fetch → skeleton → real values, so the section keeps its shape across hide/show without jumping. The `Hide credentials` action lives below the table in the revealed state, keeping the toggle in a single consistent location.
- **Danger Zone** uses a custom uppercase mono header with a `TriangleAlert` lucide icon, set apart from the default `FormSection` heading style to mark it as a different category. The `DangerZoneItem` row now stacks (title above button) on small screens.

**Chip primitive**
- Stripped raw white background; now uses border-only chrome with semantic foreground (`text-foreground`), removing a dark-mode override and matching the visual weight of inline tags.

**Empty states**
- Unified the empty-state pattern across Knowledge stores list, Agents dashboard, and the Agent bindings card to match the variables-table treatment in SecretsSettings: dashed `border-border`, no filled icon container, smaller `text-sm` title / `text-xs` subtitle. The previous `bg-border` icon tile read as too heavy on dark surfaces.

## Bug fixes

- `ProviderIcon` was reading `useTheme()`, which returns `"auto"` for users on system preference. The dark-variant SVG only loaded when theme was explicitly set to dark, so users on auto saw the light-variant icon on a dark surface (most visible on Pinecone, which is dark-on-light by default). Switched to `useResolvedTheme()`, which resolves auto → light/dark, and removed the compensating `dark:brightness-150` filter.

## Migration

None.
