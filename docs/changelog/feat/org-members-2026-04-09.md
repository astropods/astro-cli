# Organization Member Management

## Summary

Adds full organization member management with proper invitation lifecycle, optimistic UI updates, role-based permissions, and consistency fixes across the organization settings pages.

## Design

### Member & Invitation Lifecycle

The members settings page now shows pending invitations alongside active members. A new `include_pending` query parameter on `GET /accounts/:account/members` lets callers opt in to seeing WorkOS memberships that have not yet synced to the local database. This is used only on the members settings page; other consumers (dashboard member count, leave-org dialog) are unaffected.

When invitations are sent, `useCreateInvitations` optimistically inserts pending rows into the members cache so the table updates instantly. On settle, the real server data replaces the optimistic entries.

### Invitation Revocation

`useRemoveAccountMember` now invalidates both `members` and `invitations` queries on success so the table refreshes immediately after removal. On the server, `Sync.RemoveMember` was extended to handle members that only exist in WorkOS (no local DB row) by falling back to `findMembershipForUser`. When removing a pending member, any outstanding WorkOS invitation is also revoked so the invite link cannot be re-accepted.

### Event Consumer Fix

The WorkOS event consumer previously created local DB entries for all membership events including pending ones. This caused organizations to appear in a user's account list before they accepted the invitation. The consumer now skips non-active memberships; the `organization_membership.updated` event creates the local entry when the membership transitions to active. A one-time cleanup script (`cmd/cleanup-pending-members`) removes stale rows created before this fix.

### Role-Based Permissions

Organization settings pages now enforce role-based visibility:

- **Secrets & Variables** nav item hidden for non-admin members
- **Invite members** button gated behind admin role
- **Profile editing** (avatar, display name), **username change**, and **org deletion** are disabled for non-admins with permission tooltips explaining why
- **Leave organization** remains available to all members

All of these are backed by server-side permission checks (`org:admin` for account mutations, `org:manage` for invitations and member management).

### UI Polish

- Organization listing page now renders actual account avatars via `UserAvatar` instead of generic building icons, with a thin border ring
- Removed unused `avatar_version` plumbing from member responses and profile editor

## Migration

No database migration required. Run `cmd/cleanup-pending-members` once against the database to remove stale pending member rows created before the event consumer fix:

```
DATABASE_URL=... WORKOS_API_KEY=... go run ./cmd/cleanup-pending-members
```

Use `DRY_RUN=true` to preview changes first.
