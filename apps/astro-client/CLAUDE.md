# astro-client conventions

## Styling

**Always use `className` with Tailwind utilities. Never use `style={}`** except for two cases where Tailwind has no equivalent:
- `clamp()` / `calc()` expressions (e.g. responsive padding like `clamp(16px, 4vw, 108px)`)
- Runtime-computed colors using `color-mix()` or dynamic CSS vars (e.g. status badge backgrounds)

Use `cn()` from `@/lib/utils` for conditional or merged class strings.

### Typography

Use semantic text-size classes from the theme rather than arbitrary sizes:

| Class | Use |
|---|---|
| `text-heading-4` | Component headings, tab labels, names |
| `text-body` | Standard body copy |
| `text-body-sm` | Secondary body copy |
| `text-label` | Mono-style labels, badges |
| `text-mono-sm` | Monospace small text |

Font families: `font-sans` (body), `font-mono` (code/labels).

## Component library

Reach for these before writing custom markup:

- **`Button`** — all interactive actions; use `variant` + `size` props
- **`InlineBadge`** — inline status/count chips
- **`StatusIndicator`** — dot + optional spinner for live status
- **`ActionPanel`** — warning/info banners with optional CTA
- **`ErrorPanel`** — error states inside cards
- **`Tooltip` / `TooltipProvider`** — any icon-only button needs a tooltip
- **`SidePanel`** — right-side slide-in panel with optional drag-to-resize; use `resizable` prop

## Side panel pattern

All right-side panels (Configure, Trace, Chat) are rendered as children of `<SidePanel>`. The panel owns the shell (`border-l`, `bg-surface`, drag handle). Panel content components are layout-only (`flex flex-col h-full w-full`) — no border or background of their own.

```tsx
<SidePanel open={panelOpen} onWidthChange={setPanelWidth}>
  {configOpen && <ConfigurePanel ... />}
  {selectedTrace && <TraceDetailPanel ... />}
  {playgroundOpen && <PlaygroundPanel ... />}
</SidePanel>
```

## Data fetching

- Never call `api.*` directly in components — use query hooks from `src/api/queries/`
- All query keys come from the factories in `src/api/queries/keys.ts`
- Mutations invalidate affected queries in `onSuccess`
