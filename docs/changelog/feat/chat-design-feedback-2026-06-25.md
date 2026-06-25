## Summary

The single-agent chat surface drifted from the product design: header icons read with mismatched colors, the conversation pane was a flat background, the empty thread showed only a bare heading, and the user bubble used the primary fill instead of the neutral bubble color from the design. This change aligns the chat with the design prototype.

## Design

The conversation pane now carries a restrained brand wash via a new `.chat-pane-bg` utility — two faint primary radial glows (top edge and bottom-right) over the page background, expressed with semantic tokens so it tracks light/dark automatically. It replaces the flat `bg-background` on the thread root.

Header iconography is unified on `text-foreground` so the History clock matches its label and the Lucide new-chat affordance reads consistently rather than appearing as a one-off color.

The empty thread state is rebuilt around agent identity: the deployment avatar plus the agent name and a "Send a message below to start a new chat" subtitle, matching the prototype. Agent identity now threads from `ChatWorkspace` through `ChatThread` into the thread view, and the avatar URL resolves through the same deployment-avatar path used by the agent menu.

The user bubble switches from the primary fill to the neutral `muted` surface (the prototype's bubble color), keeping assistant turns borderless on the canvas. The composer shell adopts the prototype's message-field treatment: a translucent surface fill over the gradient, an input-toned border, and a primary-colored focus ring.

## Migration

No action required.
