# View badge menu item

## Summary

Restores the "View badge" action that opens the agent's holographic trading card. The V2 card redesign dropped this menu item because the V2 props didn't carry the blueprint data the badge modal needs (see GH #1224). It returns on two surfaces: the kebab menu on each `DeployedAgentCard` and the `AgentIdentity` dropdown on the agent detail page. The badge modal itself is upgraded to match the post-deploy reveal — replacing the two bare Download SVG/PNG buttons with a single "Share badge" dropdown (Share on X, Share on LinkedIn, Download PNG, Download SVG).

## Design

Both surfaces follow the same pattern:

- A `shareOpen` boolean drives a `TradingCardModal` rendered alongside the existing dialogs.
- Blueprint data (needed for integration pills on the card) is fetched lazily via `useBlueprint(account, name, { enabled: shareOpen })` — the dashboard grid pays no query cost on first paint, and the detail page pays nothing until the menu item is clicked.
- `CardData` is constructed in a `useMemo` from props the surface already has. The "Deployed" stat uses `created_at` on the deployment; the QR code points at the public blueprint page; the barcode encodes the deployment ID.

`DeployedAgentCard` gains a new optional `installedAt` prop so the V2 card stays decoupled from the server-side `AgentDeployment` shape — the `DeploymentAgentCard` adapter wires `deployment.created_at` through. The card's `menuVisible` flag now also tracks `shareOpen`, so the kebab stays anchored while the modal is open.

`AgentIdentity` already has the full `AgentDeployment` object in scope, so it pulls `created_at` and `avatar_colors` directly with no prop plumbing.

`TradingCardModal` gains a "Share badge" dropdown trigger (a port of the affordance from `LiveRevealOverlay`). Share-to-network logic constructs the public blueprint URL from `data.account`/`data.name` and opens X or LinkedIn share intents in a new tab; download actions keep using the existing `astro-trading-card/browser` helpers.

## Migration

None — additive only. Existing consumers of `DeployedAgentCard` work unchanged; pass `installedAt` if you want the "Deployed" stat to appear on the badge. The `TradingCardModal` API is unchanged — only the action UI inside the modal was swapped.
