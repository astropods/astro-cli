## Summary

The New Blueprint wizard page had an unwanted global scrollbar because it was sizing itself independently with `min-h-screen` instead of participating in the app's existing flex layout.

## Design

The root `Layout` component establishes a `flex min-h-screen flex-col` container. All other pages (AgentDashboard, KnowledgeStores, etc.) use `flex-1` on their outermost div to fill the remaining height naturally. NewBlueprint was opting out of this by setting its own `min-h-screen min-h-[100dvh]`, causing content to overflow and produce a global scrollbar.

The fix joins the flex chain:
- Outer wrapper: `min-h-screen min-h-[100dvh] w-full` → `flex flex-1 flex-col overflow-hidden`
- Inner content div: `pb-40` → `pb-6`, gains `flex flex-1 flex-col overflow-hidden`
- Carousel wrapper and row gain `flex-1` so the card fills the remaining height
- Card: `min-h-[460px]` → `flex-1`

No new abstractions, no `useEffect` body hacks — just the flex chain doing its job.

## Migration

None required.
