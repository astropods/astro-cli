# astro-client conventions

## Styling

**Always use `className` with Tailwind utilities. Never use `style={}`** except for two cases where Tailwind has no equivalent:
- `clamp()` / `calc()` expressions (e.g. responsive padding like `clamp(16px, 4vw, 108px)`)
- Runtime-computed colors using `color-mix()` or dynamic CSS vars (e.g. status badge backgrounds)

Use `cn()` from `@/lib/utils` for conditional or merged class strings.

### Colors

Always use **semantic tokens** from `@astropods/theme`. They flip across light/dark automatically; raw palette utilities (`bg-white`, `bg-stone-*`, `text-stone-*`, `text-green-*`, etc.) are forbidden in component code by the `local-theme/no-raw-theme-colors` ESLint rule.

**Elevation ladder** — pick the lightest level that visually separates from its parent:

| Token | Tailwind class | Use |
|---|---|---|
| `background` | `bg-background` | Page chrome (rare) |
| `surface` | `bg-surface` | Page body / panels (already on `<body>`) |
| `card` | `bg-card` | Lifted tiles (cards, list items) |
| `popover` | `bg-popover` | Menus, dropdowns |

**Foregrounds**: `text-foreground` (primary), `text-muted-foreground` (secondary), `text-faint-foreground` (uppercase labels, captions). Never pair raw greys with foreground tokens.

**`<Card>` primitive** — for any tile that needs the card chrome, import `Card` from `@/components/ui/card` rather than rolling `border border-border bg-card rounded-…` by hand. The primitive applies the rounded `bg-card` chrome and forwards everything else through `className`:

```tsx
<Card className="p-4">…body padding…</Card>
<Card className="p-[12px_14px] h-[100px]">…tighter tile…</Card>
```

If you need a different elevation (`bg-surface`, `bg-popover`) or a borderless variant, drop `<Card>` and compose the semantic tokens directly — don't override `bg-*` on `<Card>` itself.

If you genuinely need a raw color (e.g. brand chrome that doesn't theme), pair it with a `dark:` variant on the same element — the lint rule recognises that as an explicit two-mode decision. Otherwise, use a semantic token.

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

- **`Card`** — any tile / lifted surface; rounded `bg-card` chrome, padding via `className`
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
