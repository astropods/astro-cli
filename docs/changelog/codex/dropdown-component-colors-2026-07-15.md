# Summary

Tune the shared surface tokens used by dropdowns, selects, and elevated overlays so dark mode has a clearer visual hierarchy without forcing component call sites to pick raw palette colors.

# Design

Dark mode now treats `background`, `surface`, `card`, and `popover` as separate semantic elevations. The base canvas remains anchored at slate-950, while lifted surfaces interpolate toward slate-900 so app surfaces, cards, and popovers read as distinct layers on common displays. Hover and highlighted rows continue to use `bg-accent`, but the dark accent token now sits above the popover surface instead of below it.

Light mode keeps white overlay surfaces, but softens `bg-accent` with a white/slate mix so highlighted rows are visible without feeling too heavy. Dropdown, select, and outline button hover fills explicitly map to the shared accent tokens, while selected items keep their indicator treatment rather than a persistent filled row. Links, inline brand affordances, and selected-item checkmarks use a dedicated `foreground-accent` token so dark mode can keep a brighter foreground color without weakening `primary` button fills. Default primary buttons keep contrast-safe hover and active states by moving darker from the base fill.

# Migration

No user action is required. Components should continue using semantic utilities such as `bg-popover` and `bg-accent` instead of raw slate classes.
