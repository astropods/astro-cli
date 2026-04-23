# Blueprint draft card and setup wizard polish

## Summary

Visual refinements to the blueprint draft card and the "Setup Your Agent Blueprint" wizard's review/publishing steps. Draft cards needed clearer differentiation from published cards, and the scanner animation on the review step used hardcoded teal colors that didn't adapt to different color schemes.

## Design

**Draft card treatment** — Draft blueprint cards now use a translucent background (25% opacity white/black) instead of a near-opaque neutral, have no outer solid border (only the inner dashed border remains, flush to the card edge), no corner radius, and no shadow. Together these changes give drafts a distinctly "unfinished" feel compared to the filled, shadowed, accent-colored published cards.

**Scanner overlay** — The review step's scan line and corner registration marks switched from a fixed `--color-teal-500` palette to white with `mix-blend-overlay`. This keeps the animation color-neutral across themes; on dark avatars the marks brighten, on light avatars they fade gracefully.

**Avatar border-radius** — Both the publishing-step and review-step avatar containers were reduced from `rounded-2xl` (16px) to `rounded-md` (6px) for a tighter, less bubbly look.

## Migration

No migration required.
