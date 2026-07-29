# Summary

Clarify surface polish after the recent theme token updates without carrying forward broad semantic background experiments that made blueprint detail pages feel visually off.

# Design

The semantic background ladder is reset to the production-style values so existing surfaces keep their expected baseline:

- Light mode keeps `surface` at `slate-100`, `card` and `popover` at white, and `muted` at `slate-200`.
- Dark mode keeps the production page `background` at slate-950 and preserves the production `muted` alias to `card`, with `surface` at a 70/30 slate-950/900 mix, `card` at 60/40, and `popover` at a 35/65 slate-950/900 mix.
- Inputs and selectors keep the production field fill: white in light mode and slate-950 in dark mode, so field chrome does not inherit the raised `card` tone.
- Draft blueprint cards use a low-opacity slate tint to distinguish the finish-setup state without giving it the stronger accent treatment of deployable blueprints.
- Components that needed targeted polish, including selected pills, trace detail sections, blueprint detail cards, warning panels, log toolbar controls, chat file rows, and chat links, now use semantic tokens or localized treatment instead of raw colors.
- Theme Storybook labels document the reset background values plus the added foreground/status tokens.

# Migration

No user action is required.
