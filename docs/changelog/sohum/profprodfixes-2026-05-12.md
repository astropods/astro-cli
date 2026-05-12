# Profile production fixes

## Summary

Five bugs on the account profile page: org admins and members had no access to the internal view, handleless members rendered as bare `@` links, the onboarding form gave no feedback when the username field was left empty, clicking an org profile blocked navigation until all data finished loading, and skeleton loaders duplicated markup instead of reusing existing components.

## Design

### Org member permission model

Previously the internal view was gated solely on `isSelf` — the viewer's account ID matching the profile's account ID. For org profiles no individual ever satisfies this, so org members saw nothing but the public view regardless of their role.

The fix introduces two derived flags:

- `isOrgMember` — the authenticated user appears in `membersData` with any role
- `isOrgAdmin` — same check but role is `admin` or `owner` (matching the pattern in `LeaveOrganizationDialog` and `OrgMembersSettings`)

These combine with `isSelf` into two permission levels:

| Flag | Value | Gates |
|---|---|---|
| `canViewDeployments` | `isSelf \|\| isOrgMember` | Agents tab, sidebar agent count, `useDeployments` query enabled |
| `isOwnerOrAdmin` | `isSelf \|\| isOrgAdmin` | Customize Order, Edit profile, View as visitor |

Private blueprints and the visibility filter dropdown are shown to any org member (`isInternalView = canViewDeployments && !isVisitorMode`). Only edit profile and view-as-visitor remain behind `isAdminView = isOwnerOrAdmin && !isVisitorMode`.

To avoid a flash of the public view while the members query resolves, the loader now fetches members alongside `getAccount` for org profiles. Personal profiles still resolve `isSelf` synchronously from the auth context so they are unaffected.

### Handleless members

`AccountMember.username` is an empty string for invited users who haven't completed onboarding. These members rendered as `@` links pointing to `/{empty-string}` — the blueprints index. The `activeMembers` filter now requires `!!m.username`, which also trims the "View all" count and the member dialog list correctly.

### Username required on onboarding

The submit button was disabled without a valid username but no message explained why. `AccountNameInput` gains an `onBlur` prop; the `value.length > 0` guard on the error div is dropped (safe — all other callers derive `displayError` from `useAccountNameValidation`, which returns `null` for empty inputs). `Onboarding.tsx` sets a `nameTouched` flag on blur and renders "Username is required" when touched and empty.

### Optimistic profile navigation

The route loader previously awaited six parallel endpoints before React Router committed the navigation, blocking the transition for the full round-trip. The loader now only awaits `getAccount` (plus `getAccountMembers` for org profiles to resolve permissions without a flash). Blueprints, deployments, and hearts are client-fetched by TanStack Query with skeleton states while they load.

Skeletons reuse `BlueprintCardSkeleton` (exported from `BlueprintListView`) and `AgentCardSkeleton` (exported from `DeployedAgentsSection`) so the loading UI is pixel-identical to the rest of the product. Sidebar stat cells pulse with a skeleton pill while counts are pending.

## Migration

No action required.
