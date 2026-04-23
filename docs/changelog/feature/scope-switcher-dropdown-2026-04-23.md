---
title: Scope switcher — form-field dropdown style + repositioned to action area
---

## Summary

Updates the scope switcher on Blueprints, Agents, and Knowledge Stores to use the design system's form-field dropdown aesthetic (white background, border, focus ring) instead of a plain transparent button. Moves it from the page title adornment to the right-side action area, aligned with existing CTAs. Also adds a hover animation to the Explore button in the nav.

## Design

**`OrgSwitcher`** is rebuilt as a `DropdownMenuTrigger`-wrapped `<button>` styled with `inputBase` and `inputFocusVisible` from the input component — the same visual treatment as other form controls. The trigger shows a `UserAvatar` (18px) alongside the account username, with a chevron pinned to the right via `justify-between`. Hover applies `bg-stone-200` to signal interactivity.

The dropdown content is unchanged: accounts listed with a checkmark indicator on the active item, "Create organization" link at the bottom.

**Placement** — `PageScopeSwitcher` moves from the `adornment` prop (inline with the page `<h1>`) to the `action` prop on all three pages, where it sits to the left of primary CTAs in a `flex gap-3` row.

**Auth gating** — the entire action block (switcher + CTAs) is now consistently gated behind `isAuthenticated` on both Blueprints and Knowledge Stores, preventing CTA buttons from rendering for unauthenticated users. Agents has no CTA so `PageScopeSwitcher` handles its own auth check internally.

**Responsive** — on mobile the action row wraps to full width (`flex-wrap w-full sm:w-auto`) and the switcher stretches to fill (`w-full sm:w-48`), so buttons stack naturally rather than overflow.

**Explore animation** — the `Telescope` icon in the nav's Explore button gains a `group-hover:rotate-12` transform with a 300ms transition. Applied to all three button instances (desktop text, desktop icon-only, mobile sheet).

## Migration

No changes required.
