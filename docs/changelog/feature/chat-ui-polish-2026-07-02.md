# Chat UI polish

## Summary

A pass of visual and interaction polish across the chat surface — the
conversation view, header, composer, conversation history, and the live
streaming indicator. The goal was to bring chat up to the app's baseline for
spacing, radius, and light/dark parity, and to fix a set of small interaction
regressions rather than change any behavior or data flow.

## Design

- **Streaming indicator** — the "agent is replying" pulse renders as an
  `animate-ping` ring behind the deployment avatar. The ring color is driven by
  the semantic `--primary` token; because `animate-ping` fades opacity toward
  zero, a single low-alpha value that reads on white disappears on the dark
  panel. The ring now carries a dark-mode alpha override so it stays visible on
  both surfaces without touching the token.
- **Header and layout** — header spacing, a transparent header treatment, and a
  full-width gradient align the chat frame with the rest of the app; the mobile
  flyout and welcome avatar were matched to the same system.
- **Composer and copy affordance** — the copy button no longer overlaps the
  composer, and the typing indicator uses the app's smaller radius so the pulse
  no longer overlaps its own text.
- **Conversation history** — rename and delete are handled inline with inline
  confirmation, and the actions are always shown on touch devices where hover
  is unavailable.
- **Cross-app consistency** — the chat icon button and its tooltip adopt the
  shared radius, and ghost-button hover is now visible on light-mode surfaces.
- **Deployment menu cleanup** — the agent deployment menu dropped its unused
  `header` variant (never wired up since the initial chat import; both call
  sites use `detail`), removing a dead branch that relied on raw palette colors
  in favor of the single semantic trigger.

## Migration

None. These are presentational and interaction changes only; no API, data, or
configuration changes.
