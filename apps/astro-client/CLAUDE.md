# astro-client conventions

## Styling

**Always use `className` with Tailwind utilities. Use `style={{}}` only for
a value the Tailwind JIT scanner can't statically resolve into a class** —
a genuinely runtime-computed value, not a hardcoded one. Two common shapes
of that: `clamp()`/`calc()` expressions (e.g. responsive padding like
`clamp(16px, 4vw, 108px)`), and a CSS function referencing a runtime or
theme value (`color-mix()`, `var(--foo)`, a computed gradient). Those two
aren't the *only* legitimate cases, they're the two most common; the test
is always "can this be a static class," not "is it on this list."
*Enforced (partially):* `local-theme/no-static-inline-style` is `warn` and
flags a literal-valued `style={{}}` property for a small set of CSS
properties Tailwind definitely covers (`display`, `width`, `height`,
`color`, `background`, `backgroundColor`, `flexGrow`, `fontSize`) — it
can't catch every possible lazy inline style, only that narrow, low-false-
positive slice. CI fails if that count grows past baseline (4 as of
2026-08-26) — see `scripts/check-inline-style-budget.mjs`. If you fix one,
the script prints a reminder (doesn't fail) to lower the baseline in the
same PR, so the win doesn't quietly get available as slack again later.

Use `cn()` from `@/lib/utils` for conditional or merged class strings.

### Colors

Always use **semantic tokens** from `@astropods/theme`. They flip across light/dark automatically; raw palette utilities (`bg-white`, `bg-stone-*`, `text-stone-*`, `text-green-*`, etc.) are forbidden in component code by the `local-theme/no-raw-theme-colors` ESLint rule. *Enforced:* the rule is `warn`, not `error` (60 pre-existing violations as of 2026-08-26 aren't fixed yet), but CI fails if the violation count grows past that baseline — see `scripts/check-theme-lint-budget.mjs`. Fix a violation opportunistically when you're already in that file, and lower the baseline in the same PR; the script reminds you (doesn't fail) if you fix one and forget to.

**Elevation ladder** — pick the lightest level that visually separates from its parent:

| Token | Tailwind class | Use |
|---|---|---|
| `background` | `bg-background` | Page chrome (rare) |
| `surface` | `bg-surface` | Page body / panels (already on `<body>`) |
| `card` | `bg-card` | Lifted tiles (cards, list items) |
| `popover` | `bg-popover` | Menus, dropdowns |

**Foregrounds**: `text-foreground` (primary), `text-muted-foreground` (secondary), `text-faint-foreground` (uppercase labels, captions), `text-foreground-accent` (links/icons). Reserve `bg-primary text-primary-foreground` for filled primary actions.

**`<Card>` primitive** — for any tile that needs the card chrome, import `Card` from `@/components/ui/card` rather than rolling `border border-border bg-card rounded-…` by hand. The primitive applies the rounded `bg-card` chrome and forwards everything else through `className`. *Enforced (ratchet, not a lint rule):* as of 2026-08-26 only 9 files import `Card` while 30 lines still hand-roll the same `bg-card`+`border`+`rounded-*` combination — a real AST-based lint rule for this would false-positive on legitimate `bg-card` usage that isn't card chrome, so instead a grep-based count (same heuristic, deliberately coarse) is ratcheted in CI — see `scripts/check-card-chrome-budget.mjs`. Prefer `<Card>` for new code regardless of what's nearby; fix a violation opportunistically and lower the baseline in the same PR — the script reminds you (doesn't fail) if you fix one and forget to.

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

## Component props

Type props with an interface, not a `type` alias. `interface XProps` is the dominant pattern in this codebase; reach for `type` only when an interface can't express the shape:
- a discriminated union (`type RequestIncreaseDialogProps = (FixedProps | PickerProps) & {...}`)
- props derived from an existing type via `React.ComponentProps<typeof X>` or `VariantProps<>` (the shadcn-style primitives in `components/ui`)

```tsx
export interface BlueprintDetailHeaderProps {
  account: string;
  name: string;
  categories: string[];
  canEdit?: boolean;
  isDraft?: boolean;
  onArchive?: () => void;
  avatarUrl?: string;
}

export function BlueprintDetailHeader({ account, name, categories, canEdit = false, isDraft = false, onArchive, avatarUrl }: BlueprintDetailHeaderProps) { ... }
```

- Mark optional props with `?` and default them in the destructure, not with `defaultProps`.
- Name boolean props as a question: `isDraft`, `canEdit`, `isPending`.
- Name event-handler props `on` + verb, taking the new value as its argument: `onOpenChange(open: boolean)`, `onArchive()`, `onSort(key)`.
- Type children as `children: React.ReactNode`.
- Export the `Props` interface only when something outside the file needs it — a story, a test, or another component importing the type (`SidebarCardProps`, re-exported from `blueprint-detail/index.ts`). A page-local helper component (e.g. `TabSearchInputProps` in `AccountProfile/TabToolbar.tsx`) keeps its `Props` unexported.

A few files use a plain (non-union, non-derived) `type XProps = {...}` for no structural reason (e.g. `new-blueprint/LinkConfirmDialog.tsx`) — that's a one-off, not a second convention to copy.

## Component file structure

- One component per file, named after the component (`BlueprintDetailHeader.tsx`) — there are no per-component `ComponentName/index.tsx` folders anywhere in `src`.
- A `.test.tsx` sits next to the component it tests, same name, same folder (`BlueprintDetailHeader.tsx` + `BlueprintDetailHeader.test.tsx`).
- When a feature grows past one component, split it into a flat folder of sibling files rather than nesting — `components/blueprint-detail/` holds `BlueprintDetailHeader.tsx`, `BlueprintDetailContent.tsx`, `BlueprintDetailSidebar.tsx`, `SidebarAuthor.tsx`, etc., each with its own test. A small subcomponent tightly coupled to its parent (`SidebarCard` inside `BlueprintDetailSidebar.tsx`) can share the parent's file instead of getting its own; split it out once it's reused elsewhere.
- Reusable feature components live in `src/components/<feature>/` (kebab-case: `blueprint-detail`, `agent-detail`, `account-profile`); page-specific components live in `src/pages/<Route>/` next to the route's entry component.
- Don't add a barrel `index.ts` re-exporting a folder's components. `blueprint-detail/index.ts` is the only one in the codebase, and even its own Storybook stories bypass it and import each file directly. Import from the specific file: `@/components/blueprint-detail/SidebarAuthor`.
- `src/pages/<Route>` folder casing is inconsistent: kebab-case is more common (`agent-detail`, `settings`, `blueprints`, `chat`, `configure`) but a few use PascalCase matching the route component (`AccountProfile`, `knowledge/KnowledgeStoreDetail`). Match whatever the folder you're in already uses.

## Side panel pattern

Right-side panels (`TraceDetailPanel`, `PodDetailPanel`, `ChatWorkspace`) are rendered as children of `<SidePanel>`. The panel owns the shell (`border-l`, `bg-surface`, drag handle). Panel content components are layout-only (`flex flex-col h-full w-full`) — no border or background of their own. Configure is a full page (`AgentConfigure.tsx`), not a side panel.

```tsx
<SidePanel open={panelOpen} onWidthChange={setPanelWidth}>
  {selectedTrace && <TraceDetailPanel ... />}
</SidePanel>
```

## Data fetching

- Never call `api.*` directly in components — use query hooks from `src/api/queries/`
- All query keys come from the factories in `src/api/queries/keys.ts`
- Mutations invalidate affected queries in `onSuccess`
- Always handle `isError`/`isLoadingError` alongside `isLoading` — see [docs/04-guides/frontend-loading-states.md](../../docs/04-guides/frontend-loading-states.md)
