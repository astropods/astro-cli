# Organizations & RBAC Implementation

This document covers the full scope of the organizations feature branch, including WorkOS integration, RBAC, membership management, agent visibility, and sync architecture.

## 1. WorkOS Dashboard RBAC Configuration

Configure these roles and permissions in your WorkOS Dashboard under **Organizations > Roles**.

### Roles

| Role   | Slug     | Permissions                                                              |
|--------|----------|--------------------------------------------------------------------------|
| Owner  | `owner`  | `agents:read`, `agents:write`, `agents:deploy`, `org:manage`, `org:admin` |
| Admin  | `admin`  | `agents:read`, `agents:write`, `agents:deploy`, `org:manage`              |
| Member | `member` | `agents:read`, `agents:deploy`                                            |

### Permissions

| Slug            | Description                             |
|-----------------|-----------------------------------------|
| `agents:read`   | View agents, versions, configs          |
| `agents:write`  | Push (register) agents, set visibility  |
| `agents:deploy` | Deploy and undeploy agents              |
| `org:manage`    | Manage members and invitations          |
| `org:admin`     | Rename/delete org, billing              |

### Setup Steps

1. Go to **WorkOS Dashboard > Organizations > Roles**
2. Create each role above with the listed permission slugs
3. Ensure `owner` is set as the default role for organization creators
4. Verify that the WorkOS environment variables are set:
   - `WORKOS_API_KEY` — your WorkOS API key
   - `WORKOS_CLIENT_ID` — your WorkOS client ID

## 2. Permission Enforcement

### `RequireAccountPermission` Middleware

Replaces the old `RequireAccountRole` middleware (`middleware/account.go`). Two authorization paths:

1. **Personal accounts** — any member has all permissions implicitly; no JWT check needed.

2. **Organization accounts** — the session JWT must be scoped to the target org (`session.OrganizationID == account.WorkOSOrganizationID`). Clients must call `POST /auth/switch-org` before accessing an org's resources. Permissions are read directly from the JWT `permissions` claim — no DB lookup needed. If the JWT is scoped to a different org, the request is rejected with 403.

### Route Protection

| Route Group | Permission | Description |
|---|---|---|
| `PUT /accounts/:account` | `org:admin` | Rename account |
| `GET/POST/PUT/DELETE /accounts/:account/members` | `org:manage` | Member CRUD |
| `GET/POST/DELETE /accounts/:account/invitations` | `org:manage` | Invitation CRUD |
| `POST /agents/:account/:name/register` | `agents:write` | Register agent build |
| `PUT /agents/:account/:name/visibility` | `agents:write` | Set public/private |

## 3. Organization Account Lifecycle

When a user creates an account with `type: "organization"`:

1. Local `accounts` row is created (Astro is source of truth for account identity).
2. WorkOS Organization is created with `external_id = account.id`.
3. `accounts.workos_org_id` is set to link the two.
4. WorkOS membership is created for the creator as `owner`.
5. Local `account_members` row is updated with `workos_membership_id`.

Each step has compensating actions on failure (delete local account / delete WorkOS org).

## 4. Membership Sync Architecture

Memberships are kept in sync between Astro's `account_members` table and WorkOS in two directions.

### Write Path (Astro → WorkOS): `org.Sync`

All member mutations go through `org.Sync` which writes to WorkOS first, then to the local DB. If the local write fails, a compensating action cleans up WorkOS. `ChangeMemberRole` only updates WorkOS (no local role column). Methods: `AddMember`, `ChangeMemberRole`, `RemoveMember`.

### Read Path (WorkOS → Astro): `org.EventsConsumer`

A dedicated worker process (`SERVER_MODE=worker`) polls the WorkOS Events API for `organization_membership.created`, `.updated`, and `.deleted` events. It upserts or removes local `account_members` rows accordingly (membership presence only — roles are not stored locally). A `workos_event_cursor` table tracks the cursor for idempotent polling.

### Login-Time Reconciliation

On login and token refresh, `SyncMembershipsForUser` lists all active WorkOS memberships for the user and upserts them locally. This catches any events missed between polls.

## 5. Organization Context Switching

`POST /auth/switch-org` refreshes the session JWT scoped to a specific WorkOS organization. The new JWT carries `role` and `permissions` claims for that org. The sealed session cookie is updated with the new tokens.

This allows the frontend to switch between orgs without a full re-login.

## 6. Agent Visibility Model

**Replaces the previous semver publish model.** Key changes:

- The `agent_published_versions` table is dropped entirely.
- Agents now have a `visibility` column (`public` or `private`, default `private`).
- `PUT /agents/:account/:name/visibility` toggles visibility (requires `agents:write`).
- `GET /api/v1/agents` (public catalog) returns agents where `visibility = 'public'`, showing their latest version.
- `GET /api/v1/agents/:account/:name` returns the agent if it's public, or if the caller is an account member. Private agents return 404 to non-members.
- The `Version` field (semver tag) is removed from agent version responses.
- The `ListPublic()` / `GetPublishedVersionsForAgent()` / `Publish()` / `compareSemver()` functions are removed from the agent index.

## 7. Schema Changes

### `account_organizations` table (new)
- Maps organization accounts to WorkOS: `account_id` (PK, FK → accounts) + `workos_org_id` (NOT NULL, UNIQUE).

### `account_member_workos` table (new)
- Maps org members to WorkOS memberships: `(account_id, user_id)` (PK, FK → account_members) + `workos_membership_id` (NOT NULL, UNIQUE).

### `agents` table
- Added `visibility varchar(10) NOT NULL DEFAULT 'private'` with a partial index on `visibility = 'public'`.

### New table: `workos_event_cursor`
- Singleton table (`id = 1`) storing the WorkOS events cursor for the polling consumer.

### Dropped table: `agent_published_versions`
- No longer needed; agent visibility replaces semver publishing.

## 8. New API Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/auth/switch-org` | Session cookie | Switch JWT org scope |
| `GET` | `/accounts/:account/members` | `org:manage` | List members |
| `POST` | `/accounts/:account/members` | `org:manage` | Add member |
| `PUT` | `/accounts/:account/members/:user_id` | `org:manage` | Change member role |
| `DELETE` | `/accounts/:account/members/:user_id` | `org:manage` | Remove member |
| `GET` | `/accounts/:account/invitations` | `org:manage` | List invitations |
| `POST` | `/accounts/:account/invitations` | `org:manage` | Send invitation |
| `DELETE` | `/accounts/:account/invitations/:id` | `org:manage` | Revoke invitation |
| `PUT` | `/agents/:account/:name/visibility` | `agents:write` | Set agent visibility |

## 9. New Internal Packages

### `internal/org`
- `Client` — wraps WorkOS Organizations, Memberships, Invitations, and Roles SDK calls.
- `Sync` — write-path sync logic (Astro → WorkOS) for memberships.
- `EventsConsumer` — polls WorkOS Events API for reverse sync (WorkOS → Astro).
- `types.go` — Organization, Membership, Invitation, Role types.
