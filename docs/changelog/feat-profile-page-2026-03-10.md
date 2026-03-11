# Profile page with agent templates and privacy indicators

## Summary

The account profile page now works for both personal and organization accounts, showing installed agents and published agent templates. A new API endpoint scopes agent listing to a single account, and a shared privacy badge indicates private agents across the UI.

## Design

- **Profile link** — Added a "Profile" item to the user dropdown menu linking to `/:account` for the personal account.
- **Unified profile page** — The `/:account` route now renders the same layout for personal and org accounts: large avatar, full name, @username, installed agents (member-gated with a shield tooltip), and agent templates.
- **Account-scoped agent API** — New `GET /api/v1/agents/:account` endpoint uses the existing `ListForAccount` index method. Public agents are visible to all; private agents are filtered unless the caller is an account member.
- **PrivacyBadge component** — Shared component (`src/components/PrivacyBadge.tsx`) renders a lock icon with "Private" label and a delayed tooltip. Used on both `AgentCard` and `AgentDetailContent` with `className` and `onClick` props for context-specific styling.
- **Card styling** — `DeployedAgentCard` border updated to match `AgentCard` visual treatment (stone-400 border, teal hover, shadow).

## Migration

No migration required.
