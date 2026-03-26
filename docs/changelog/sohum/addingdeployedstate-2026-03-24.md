# First-Deploy Reveal Flow on My Agents

## Summary

Refined the post-deploy experience so first-time deploy launches present a persistent reveal moment on `My Agents`, while preserving predictable navigation to deployment detail and back. This shifts the reveal from an in-detail transition to an entry-point experience tied to the initial deploy action.

## Design

- **Reveal placement and timing:** The layered reveal now appears immediately after the first configure-page deploy action by routing into `My Agents` with a deployment handoff marker, instead of waiting for deployment-detail state transitions.
- **Persistent user-controlled overlay:** The reveal remains visible until explicit user action (`View deployment` or dismiss), eliminating auto-close behavior caused by background polling and refresh cycles.
- **One-time semantics per deployment:** A deployment-scoped local storage key ensures the reveal is only shown once for that deployment ID, even when users navigate back from deployment detail.
- **Navigation contract:** `View deployment` opens deployment detail with `from=agents`, and back navigation returns users to `My Agents` rather than configure screens, keeping the flow coherent after the reveal handoff.
- **Copy and status language:** Reveal copy now communicates deployment-in-progress state (`DEPLOYING`, `is deploying!`, `View deployment`) and uses the existing warning/yellow token styling for visual consistency with pending status patterns.
- **Share target reliability:** Reveal share links point at the public share route (`/share/:account/:agentSlug`) so production social unfurls resolve against OG-tagged pages.

## Migration

No migration required.
