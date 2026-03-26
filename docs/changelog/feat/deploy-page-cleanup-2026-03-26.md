# feat/deploy-page-cleanup — Decompose DeploymentsTab and Consolidate Patterns

## Summary

The deployed agent detail page's `DeploymentsTab.tsx` was a 1,194-line monolith with 97 inline style objects, 13 local helper functions, and duplicated patterns. This change decomposes it into focused, single-responsibility components and hooks, converts all inline styles to Tailwind, and consolidates the copy-to-clipboard pattern used across 7 files into a shared hook and component.

No user-facing behavior changes. All modifications are structural and organizational.

## Design

### Before → After: File Structure

```
BEFORE (1 file, 1,194 lines):
  DeploymentsTab.tsx    — 2 components, 13 helpers, all inline styles

AFTER (15 files, largest 211 lines):
  DeploymentsTab.tsx (211 lines)           — orchestration: summary cards, data wiring
  ActiveContainerAccordion.tsx (197 lines)  — accordion header + tabs for logs/vars/domains
  DeploymentHistoryTable.tsx (186 lines)    — history grid, current + past rows, services
  LogViewer.tsx (166 lines)                 — log toolbar + output, owns query + filter state
  DeploymentHistoryRow.tsx (105 lines)      — shared row for current and past deployments
  EnvVarsPanel.tsx (60 lines)              — env vars tab with sensitive masking
  DomainsPanel.tsx (33 lines)              — domains tab
  history/utils.ts (78 lines)              — duration, status mapping helpers
  use-log-filtering.ts (44 lines)          — single-pass log filter + count hook
  use-compact-layout.ts (25 lines)         — responsive breakpoint hook (replaces use-mobile.ts)
  use-container-selection.ts (21 lines)    — container sync hook
  use-copy-to-clipboard.ts (22 lines)      — clipboard write + feedback state + cleanup
  copy-button.tsx (33 lines)               — self-contained copy icon button component
  clipboard.ts (25 lines)                  — clipboard API with legacy fallback
  log-utils.ts (32 lines)                  — log parsing and coloring utilities
  env-utils.ts (23 lines)                  — sensitive env var detection
```

### Inline Styles → Tailwind

All 97 `style={{}}` objects replaced with Tailwind utility classes via `cn()`. The `C`, `S`, `T`, `I` constant objects (color palette, font families, typography scales, icon sizes) mapped to existing theme tokens (`text-foreground`, `bg-surface`, `text-heading-4`, `font-mono`, etc.) and deleted.

### Copy-to-Clipboard Consolidation

Seven files duplicated the same pattern: `useState(false)` + `navigator.clipboard.writeText()` + `setTimeout(() => setCopied(false), N)`. Replaced with:

- **`useCopyToClipboard(resetMs?)`** — encapsulates clipboard write (with legacy fallback), boolean feedback state, timeout, and cleanup on unmount.
- **`<CopyButton copyText={...} />`** — self-contained icon button accepting a string or `() => string` for lazy evaluation.

Updated consumers: LogViewer, ActiveContainerAccordion, ExternalUrls, BlueprintDetailBreadcrumb, DeployedAgentCard, KebabMenu, MonitorTab.

### Functional Fixes (non-breaking)

These are minor behavioral corrections, not feature changes:

- **Button-in-button nesting**: The accordion header was a `<button>` containing the playground command copy `<button>`. Changed the outer element to a `<div>` with `onClick` to fix the HTML nesting violation that caused hydration errors.
- **Log regex inconsistency**: `log-utils.ts` matched `exception` for coloring but the filter hook did not count it. Both now share the same exported regex constants.
- **Log regex partial-word matching**: `/connected/` matched "disconnected", `/attempt/` matched "attempted". Added `\b` word boundaries.
- **Redundant sensitive check**: `EnvVarsPanel` re-ran `isSensitiveEnvVar()` on every var even though the caller already set `v.secret`. Simplified to trust the pre-computed flag.
- **Hand-rolled kebab menu**: Deployment history rows used a custom absolute-positioned menu with manual click-outside handling. Replaced with the existing Radix `DropdownMenu` component.
- **Breakpoint hooks merged**: `useIsMobile` (768px) and `useCompactLayout` (1180px) were identical except for the breakpoint value. Merged into `useMediaBreakpoint(px)` with both as named convenience exports. Also fixed: uses `mql.matches` instead of `window.innerWidth`, and initializes synchronously to avoid layout flash.
- **useMemo/useEffect over-invalidation**: Dependencies on the full `deployment` object caused recomputation on every TanStack Query refetch. Narrowed to specific fields.
- **Copy timeout leak**: `setTimeout` IDs for copy feedback were not cleared on unmount. Now handled by the shared hook.

### Reuse of Existing Components

- **`StatusIndicator`** — replaced 6 inline SVG/spinner status indicators in accordion headers and history rows.
- **`DropdownMenu`** — replaced the hand-rolled kebab menu in deployment history rows.

## Migration

No changes required. All exports consumed externally (`DeploymentsTab`) remain at the same path with the same interface.
