## Summary

The chat surface had no way to inspect the agent you're talking to — its live health, recent usage, or its configured system prompt and tools — without leaving the conversation for the agent detail pages. This adds an inspector side panel to chat, opened from a new "Details" control in the chat header, matching the design prototype.

## Design

A "Details" toggle in the chat header opens a right-side inspector with two underline tabs, Overview and Settings. On desktop the panel is a floating, rounded, shadowed `bg-surface` card with a gutter that reflows the conversation to the left (matching the design prototype). Opening and closing animate smoothly (the card expands/collapses its width with a slide-and-fade, reflowing the conversation), via a two-phase mount so both enter and exit transition and the panel's queries stay idle until it is opened. On narrow viewports it slides in as a sheet, and the header's History/Details labels collapse to icons so the toggle stays reachable. Open state and the active tab live in `ChatWorkspace` so they persist while switching conversations.

The panel reuses existing data sources rather than introducing new reads:
- Overview pulls deployment record + runtime to show build, last-deployed, and per-workload health (joined spec + live K8s state, classified with the same `derivePodStatus` the Deployments tab uses), then usage stats (requests, tokens, p95, error rate) and a recent-traces list from the observability summary/traces queries. The usage trend line is the same `RequestSparkline` rendered on the org/agents cards — extracted into a shared component and fed the same per-deployment request/token series — so the two surfaces match exactly.
- Settings reads the agent's self-reported config (system prompt + tools) from the messaging sidecar's `agent/config` endpoint — the same shape the playground uses — proxied through astro-server's existing messaging proxy, so no new server route is needed. A new `getDeploymentAgentConfig` client method and `useDeploymentAgentConfig` query back it.

A footer links out to the full agent detail page for editing.

Two prototype Overview widgets are intentionally omitted because no backing data source exists yet: **7-day spend** (observability metrics carry no per-deployment cost) and **"most used skills"** (no skill-usage concept in the product). These can be added later behind real endpoints.

## Migration

No action required.
