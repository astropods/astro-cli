# Blueprint Creation & Onboarding Flow

## Summary

Introduces a full blueprint creation and onboarding experience. Users can now create a blueprint as a draft, navigate through a step-based setup flow, and arrive at the blueprint detail page with inline instructions to scaffold, configure, and push their agent. The draft state persists until the first `ast push`.

## Design

**Draft state**: A blueprint is considered a draft when `versions.length === 0`. The server creates the record immediately on `POST /blueprints`; the draft flag is derived client-side, not stored separately.

**Creation flow (`NewBlueprint`)**: Multi-step carousel (name → visibility → categories → avatar) with step transitions. On submit, calls `POST /blueprints` then redirects to `/:account/:name`. A scan/reveal animation plays over the new blueprint card before the redirect.

**Draft detail page**: `BlueprintDetailContent` receives `isDraft` and renders a "Finish setting up" panel instead of the README. The panel has three tabs — Terminal, Claude Code, Cursor — each showing copy-paste commands or a prompt to scaffold and push the agent. The sidebar disables the Deploy button while in draft state.

**Draft badge in browse/dashboard**: Blueprint cards show a "Finish setting up" yellow badge instead of a version tag when `versions.length === 0`. Clicking the card routes to `/:account/:name` (the detail page), not a separate setup URL.

**Push detection**: After `ast push`, the server sets `versions` on the blueprint. The detail page polls and transitions out of draft state automatically when the first version appears.

**Bug fixes included**:
- `BlueprintDetailSidebar`: missing `SidebarSection` import caused a runtime crash on the draft detail page
- `BlueprintDetailContent`: `React.ComponentType` referenced without `React` import
- `BlueprintDetail`: `useAuth()` was called after conditional early returns (hooks rules violation) — moved to top of component
- `AvatarPicker`: double-call bug where `onAvatarChange` was fired both directly in `navigate` and via `useEffect`

## Migration

No migration required. Existing published blueprints are unaffected. Drafts created before this change (if any) will display the finish-setup panel until their first version is pushed.
