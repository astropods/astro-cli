# Interactive rendering on the web chat

## Summary

Agents can pause a turn to ask the user something structured — approve a tool
call, fill a short form, pick from a list — instead of parsing intent back out of
free text. The sidecar already emits these blocking interactions (the Renderable
primitive); this adds the web chat's half: render the ask as a form in place of
the composer, collect the answer, and deliver it back. Inert until the feature
flag and the sidecar capability are both on.

## Design

**One domain type, three consumers.** `lib/chat/interaction.ts` defines the
`Interaction` shape (id, kind, message, JSON Schema, current value, allowed
actions, intent) plus `parseInteraction` and the response-body union. The SSE
transport, the API/cache layer, and the renderer all import from here, so the
wire contract lives in one place rather than being re-described at each boundary.

**Custom renderer, not a schema-form library.** `components/chat/interaction/`
walks the JSON Schema into typed field descriptors (`describeFields`) and renders
each with the app's own primitives — text/number inputs, textarea, a monospace
code block, switch, single- and multi-select. This keeps the form on the app's
theme tokens and Storybook, and lets `intent === "tool_permission"` present as a
permission gate (tool name as the heading, optional agent-authored message,
Approve / Deny) rather than a generic form. Required fields gate submit; the
server's schema validation surfaces as an inline error on a rejected response.

**Two sources for one pending interaction.** The live path is the `interaction`
SSE event (`onInteraction` → hook state). The durable path is
`pending_interactions` on the conversation fetch, for a reload or reconnect. The
hook prefers the live interaction and clears it once its response succeeds, so a
resolved form doesn't linger while the refetch settles.

**Composer swap.** While an interaction is pending, `ComposerSlot` replaces the
message composer with the form; submitting posts to the sidecar through the
existing catch-all messaging proxy (no new astro-server route — a proxy test pins
that contract) and, on success, clears the pending state and invalidates the
conversation.

**No flag — opt-in by the agent.** There is no platform gate on either side. An
interaction appears only when an agent chooses to call `render()`/`elicit()`, so
the feature is invisible to every chat until an author opts in, and its blast
radius is that agent's own conversations. A single misauthored interaction can at
worst wedge one conversation (the user starts a new chat to continue); nothing
crosses agents, no data is lost, and the renderer draws agent strings as escaped
text. Gating was dropped deliberately: a client flag and the sidecar capability
could not be flipped independently — a dark client while the sidecar emitted a
blocking interaction would hang the turn — so a per-flag switch was net operational
risk for a feature that already only fires on explicit agent opt-in.

## Migration

None. A chat shows an interaction only when its agent calls `render()`/`elicit()`.

## Follow-ups

Deferred, tracked for later passes:

- The interaction `message` renders as plain text; wire it through the app's
  markdown renderer where agents author formatted prompts.
- The client gates required fields only; full JSON-Schema validation for instant
  feedback (parity with the sidecar's validator) is deferred — invalid input is
  caught server-side and surfaced as an inline error on the rejected response.
- Public documentation for the interaction primitive (how an agent author triggers
  one, the supported field kinds and `x-ui` extensions, the `tool_permission`
  intent) should land under `docs-public/` (Rule #5). With no flag gating the
  feature, an agent author can use it as soon as this ships, so the docs are a
  near-term follow-up rather than a launch gate.
- No unconditional client escape from a pending interaction yet: if an agent emits
  a form with no cancel/decline/respond action and the user can't satisfy submit,
  that one conversation is wedged until abandoned. A "dismiss" affordance that
  always restores the composer would cap the worst case; recommended next.
