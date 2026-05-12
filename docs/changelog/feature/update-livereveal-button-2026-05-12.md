## Summary

Aligns the deploying-reveal overlay with the project's standard primitives so it themes correctly in light mode and inherits shared visual treatment for free.

## Design

The `LiveRevealOverlay` previously rolled its own backdrop, deploying-state badge, and outline button. Each shortcut had drift:

- The backdrop used a one-off `bg-black/[0.62] backdrop-blur-[3px]` value. In light mode this rendered as a flat grey and lost the cinematic feel of the reveal moment.
- The "Deploying" pill was a hand-rolled `<span>` keyed off the raw `--color-yellow-500` palette token. Raw palette tokens don't auto-swap between light and dark (which is why the project's lint forbids them in component code), so the pill faded into the backdrop in light theme.
- The "Share badge" button passed `variant="outline"` but kept the variant's default `bg-background`, painting an opaque pill on the dark overlay instead of an actual outlined transparent button.

Now:

- Backdrop uses `bg-black/70 backdrop-blur-md`, matching `TradingCardModal` — the same trading-card content already has this treatment elsewhere.
- The deploying pill is `<StatusBadge color="warning" indicator spinning>`, the same primitive used across the app. It themes via the semantic `--warning` token.
- The Share badge button overrides the outline variant's background to `bg-transparent` and aligns the white border/hover treatment with the close button on the same overlay.
- A stray `border-0` override on the primary "View deployment" button was dropped (no-op).

## Migration

None.
