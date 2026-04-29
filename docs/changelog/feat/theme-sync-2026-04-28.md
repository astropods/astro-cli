# Card elevation token, live theme state, and theme lint guardrails

## Summary

Dark mode regressions were piling up because the theming system had no notion
of **elevation**: components that wanted "lifted" tiles reached for `bg-white`
or `bg-surface` directly, and the body itself is `bg-surface` in dark mode —
so cards collapsed into the page chrome. Two further bugs surfaced from a
sibling defect: live experiments + theme state were stored in component-local
`useState`, so toggling the `theming` experiment from the settings page did
not reveal the theme switcher in the header until a full reload, and the live
log surface was hardcoded to `bg-white`.

This change reshapes the foundation so future theme work is mechanical rather
than archaeological: a real elevation token, a primitive component for cards,
typed semantic tokens, a module-level store for cross-tree state, and a
linter that prevents the regression from being reintroduced.

## Design

### Elevation ladder + new `card` token

The semantic-token model in `@astropods/theme` now includes a `card` token
sitting one stop above `surface`:

```
background ← page chrome
  surface  ← page body / panels (already on <body>)
    card   ← lifted tiles
    popover ← menus, dropdowns
```

In dark mode `--card` is `oklch(16% 0.011 198.266)` against a `--surface` of
`oklch(13% …)` — visibly lifted without resorting to `--background` (which is
the bluer `--color-teal-950` and reads as a different surface family).

The package exports a typed `SemanticToken` union derived from
`keyof typeof lightTheme`, and `darkTheme` is now `Record<SemanticToken, string>`
so any drift between the two themes is a TypeScript compile error caught by
`astro-theme:typecheck`.

`apps/astro-client/src/index.css` bridges `--card` and `--card-foreground`
into Tailwind via `@theme inline`, making `bg-card` / `text-card-foreground`
available as utilities.

### `<Card>` primitive

`apps/astro-client/src/components/ui/card.tsx` is a deliberately tiny
primitive: a `<div>` with the rounded `bg-card` chrome plus border, with
everything else (padding, sizing, hover state) passed through `className`.
It exists so authors stop reaching for `bg-white` / `bg-stone-*` when they
want a card; the chrome itself is the only thing the primitive owns.

```tsx
<Card className="p-4">…body padding…</Card>
<Card className="p-[12px_14px] h-[100px]">…tighter tile…</Card>
```

`MetricCard`, `MetricCardSkeleton`, and `UsageCard` now wrap `<Card>` instead
of hand-rolling `border border-border bg-* rounded-…`.

Different elevations (`bg-surface`, `bg-popover`) or borderless variants are
intentionally not propped up by `<Card>` — those compose semantic tokens
directly. The point of the primitive is to make the most common shape
(`bg-card` + border + radius) trivially reusable, not to model every tile.

### Module-level stores for theme + experiments

`useTheme` and `useExperiments` now back onto a module-level snapshot
subscribed via `useSyncExternalStore`. The previous `useState`-per-component
pattern meant that updating the value in one consumer never notified
siblings; the only path to consistency was a reload. The new pattern:

- Each setter mutates the snapshot, writes through to `localStorage`, and
  notifies all subscribers in the tab.
- A `storage` event listener still syncs across tabs.
- `useTheme` re-applies the resolved `dark` class on `<html>` whenever the
  theme changes, and listens for system-preference flips while in `auto`
  mode.

This makes "toggle Theming → switcher appears live" and any future
cross-component theme/experiment behaviour work without bespoke wiring.

### `local-theme/no-raw-theme-colors` ESLint rule

A custom rule in `apps/astro-client/eslint-rules/no-raw-theme-colors.js`
flags raw palette utilities in component code:

- `bg-white`, `bg-stone-{n}`, `bg-teal-{n}`
- `text-{stone|teal|green|coral|yellow|amber|blue}-{n}`
- `border-{stone|teal}-{n}`

A class string is allowed if it pairs the offending utility with a
`dark:<same-prefix>` modifier on the same element — that proves the author
considered both modes. Otherwise the author should use a semantic token.
The rule is registered as `error` for `src/**/*.{ts,tsx}` and is allowlisted
for stories and a couple of intentionally-literal UI primitives (`switch.tsx`,
`image-cropper.tsx`).

A single `eslint-suppressions.json` file grandfathers the ~180 existing
offenders so this PR does not require a mechanical sweep; that sweep is
deferred until the designer's palette work lands. New code is blocked
immediately.

The rule itself ships with 21 RuleTester unit tests exercising every
forbidden pattern, all the `dark:` allow paths, and template-literal /
multi-offender edge cases.

### Storybook dual-theme decorator

The Storybook preview gains a third theme global (`split`) that renders the
story twice side-by-side, each pane scoped to its own theme via a wrapper
`<div className="dark …">`. Designers and reviewers can compare both modes in
one view; selecting `light` or `dark` keeps the existing single-pane
behaviour and toggles `<html>` for a production-style render.

### Tests

- **Unit (`vitest`)** — RuleTester suite for the new ESLint rule (21 cases),
  `<Card>` primitive chrome + className passthrough (3 cases),
  `useExperiments` cross-consumer sync + persistence (3 cases), `useTheme`
  cross-consumer sync + dark-class application + persistence (3 cases).
- **Integration regression assertions** — `MetricCard.test.tsx` now asserts
  the outer container is the `<Card>` primitive (`data-slot="card"`,
  `bg-card`, no `bg-white`); `LogViewer.test.tsx` asserts the scroll
  container uses `bg-card`. Both fail loudly if either regression returns.
- **E2E (`@playwright/test`)** — `apps/astro-client/e2e/theme.spec.ts`
  covers the live-toggle path (settings → header switcher with no reload),
  dark-theme persistence across reload, and that a card on the dashboard
  has a computed `background-color` distinct from `<body>` in dark mode.

CI runs `astro-client:lint` alongside the existing `typecheck` and `test`
jobs in `.github/workflows/test.yml`.

## Migration

Nothing required — the changes are additive (new token, new primitive, new
hooks API surface) or backward-compatible (the existing `useTheme` /
`useExperiments` return shape is unchanged). The lint rule is configured to
block only new violations; existing offenders are grandfathered until the
follow-on palette-sweep PR lands.
