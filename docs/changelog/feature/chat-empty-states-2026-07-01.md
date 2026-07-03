## Summary

The chat page's empty state was a bespoke hero layout that drifted from the rest of the app and didn't cover every way a user can arrive with nothing to chat. It now reuses the `/agents` dashboard empty-state pattern (page header + dashed card) and branches on the user's actual situation so the call to action is always the right next step.

## Design

Chat is gated on deployments that expose the web messaging adapter (`isChatListEligible`). The empty state now distinguishes three cases, chosen in `Chat.tsx` from the chat-agent query:

- **No deployments at all** (`no-chat-agents`): a single "Deploy a blueprint" CTA that routes to the account's blueprints when it owns some, otherwise to Explore to find a community one (same conditional-route pattern as the chat agent switcher).
- **Deployments exist but none are chat-eligible** (`agents-not-chattable`): deploying another agent wouldn't help, so this state routes to the agents list. Copy directs the user to deploy or update a blueprint with web messaging enabled, then return to chat.
- **Eligibility reads failed** (`error`): the deployments summary or a per-account deployment fetch errored. A failed read reports zero deployments, which would otherwise masquerade as the resting "no agents" state and wrongly nudge the user to deploy a blueprint during a transient outage. This state offers a retry that refetches every underlying query instead.
- **Loading**: a branded rocket loader (`AstroLogoLoader`) with "Getting your chat ready" copy while eligibility resolves. The animated agent-mascot illustration is reserved for the resting empty states (chat "no agents", blueprints) so its looping motion no longer reads as a loading signal.

To tell "no agents" apart from "agents exist but none chattable", `useChatAgents` now also returns `totalDeployments` (a count across all the user's accounts, eligible or not) alongside the eligible `entries`. It additionally exposes `isError` (true when the summary or any per-account read failed) and a `refetch` used by the error state's retry CTA. `Chat.tsx` checks `isError` before the count-based branches precisely because an errored read looks like zero deployments.

Layout matches `/agents` exactly via `PageContainer` + `PageHeader`, so the header placement and dashed card are consistent across the two surfaces. The `loading` state is the deliberate exception: it renders the full-center branded loader without the page shell, so the "Chat" header reveals with resolved content rather than sitting above a spinner.

## Migration

No action required.
