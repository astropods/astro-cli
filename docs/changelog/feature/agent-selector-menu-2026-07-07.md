# Agent selector menu polish

## Summary

The agent switcher (shown on the agent detail page and the chat header) was cramped: account groups ran together, the currently-open agent was hidden from its own list, and the per-agent actions lived inline in the selector with no room to breathe. This tidies the switcher, surfaces the current agent, and gives the actions a dedicated home that degrades gracefully on small screens.

## Design

- **Switcher grouping** — account/org groups are separated with clear vertical spacing and muted section labels, so multi-account users can scan them apart.
- **Selected agent in list** — the open agent is no longer filtered out; it renders as a non-navigable, highlighted row with a check, consistent across the agents page and chat. A separate `hasOtherAgents` check still drives the single-agent "deploy more" affordance.
- **Actions kebab** — View blueprint, Share agent badge, Restart deployment, and Delete agent move out of the selector's inline menu into a standalone kebab beside it on desktop. The four items are defined once and rendered in whichever slot applies; the confirm/delete/badge dialogs mount once regardless.
- **Responsive fold-in** — below 1024px (where the detail tab bar collapses to a compact Deployments dropdown) the kebab disappears and the actions fold back into the selector's own dropdown via its `menuPrefix`, so nothing overlaps. The switch is driven by the existing `useMediaBreakpoint` hook because the fold-in path renders through a portal prop, not pure CSS.
- **Name truncation** — the agent name's max-width steps down through viewport breakpoints (`6/8/10/18rem`) with a matching fade mask, keeping the name clear of the right-aligned Deployments control at narrow widths.

## Migration

None.
