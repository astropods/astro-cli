## Summary

When a user has only one agent to chat with, the chat header's agent dropdown opened to an empty panel — there was nothing to switch to, and no path forward. This adds a "grow your fleet" state so a single-agent user sees their current agent as selected plus a clear prompt to deploy more agents from blueprints.

## Design

The chat agent selector reuses the shared `AgentDeploymentMenu`. Its switch list is built from the deployments summary with the current deployment filtered out, so with a single chat-eligible agent the list is empty.

`AgentDeploymentMenu` gains an optional `growFleetHref`. When it is set and the switch list is empty, the menu renders a dedicated state instead of an empty panel: the current account label, the active agent as a selected row with a checkmark, and a full-width "Deploy more agents" call to action linking to `growFleetHref`. The CTA is a bordered ghost `Button` (leading `+`) set below a separator so it reads as a distinct action rather than another selectable agent row. The account label is derived from the deployments summary the menu already reads, so no extra data was plumbed for it. When the user has multiple chat-eligible agents, the existing switch list is unchanged, and callers that omit `growFleetHref` (e.g. the agent detail page) are unaffected.

Chat wires this up by threading the active account through `ChatWorkspace` → `ChatThreadHeader`. The header computes `growFleetHref` based on whether the account still has blueprints left to deploy: it cross-references the account's blueprints against its deployments (a blueprint counts as deployed when a deployment shares its name) and points the CTA at the account's blueprints page when undeployed blueprints remain, or at Explore once they are all deployed. These lookups only run in the single-agent case, and until they resolve the CTA falls back to the blueprints page (whose empty state also routes on to Explore).

## New chat composer

The empty ("New chat") state now presents the composer as a focal element rather than a footer pinned to the bottom. While the thread is empty, the welcome block and composer are centered together as one group and the input sits taller, inviting a first message. The welcome establishes a clear hierarchy — a dominant "What should {agent} work on?" heading with a subordinate avatar and supporting subtext — so the elements read as one prompt instead of competing. Once a message is sent, the thread's `isEmpty` flips: the composer settles into its usual bottom-anchored position and shrinks back to the standard height, with the welcome block fading out and the input's min-height easing down so the shift reads as intentional motion rather than a snap. Layout state is driven entirely by assistant-ui thread state (`s.thread.isEmpty`), so no extra flags are stored.

## Migration

No action required. The prompt appears automatically in the chat agent dropdown for users with a single chat-eligible agent.
