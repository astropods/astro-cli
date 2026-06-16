## Summary

Redesigns the Connectors settings tab as an inline-row list and introduces a shared `ConnectorRow` primitive. Builds on the general settings polish PR (header spacing, `SectionHeader` refactor).

## Design

**One Connectors header, two connector rows.** GitHub and Slack previously each had their own `SectionHeader` and a per-section card. The tab now uses a single "Connectors" header followed by a list of rows under a shared `ConnectorRow` component. Each row is `border-t` separated (with `first:border-t-0` to suppress the top line above the first row) and exposes an `icon`, `name`, `description`, `action` slot, optional `isLoading` skeleton, and a `children` slot for a nested `ConnectorRowList` (orgs for GitHub, workspaces for Slack).

**Action slots.** When a connector is disconnected the action is a Connect button. When connected the action is a kebab `MoreHorizontal` menu — Reauthorize / Disconnect for GitHub, Disconnect for Slack. Slack also gets an "Add workspace" button alongside the kebab when one or more workspaces are linked.

**Per-workspace disconnect.** Removing an individual Slack workspace moves from a "Remove" text button to a 20px `Link2Off` icon button with a "Disconnect workspace" tooltip — same icon used elsewhere in the codebase for disconnect actions. The smaller button height keeps rows visually flush with GitHub's text-only org rows. The user-ID copy button is removed; it copied the raw Slack user ID, which is a different value than the `@username` shown next to it, making the affordance misleading for the common case.

**Row item typography.** `ConnectorRowItem` is `text-body-sm` so org and workspace lists read as primary content. Helper text inside the list inherits the row's size rather than overriding with the larger `text-body`.

**Copy alignment.** Slack description switched from "Talk to your agents in any connected Slack workspace" to "Message agents directly from any connected Slack workspace" so it mirrors GitHub's verb-action voice ("Build and deploy agents directly from your repositories"). Slack's disconnect-all menu item drops the redundant "Slack" suffix — the kebab is row-anchored.

## Migration

No action required.
