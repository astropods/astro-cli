# Organizations Architecture

This document explains how organizations work in Astro, covering account types, authentication, permissions, membership sync, and agent visibility.

## Account Model

Astro has two account types, both stored in the `accounts` table:

| Type | Description | WorkOS Integration |
|------|-------------|-------------------|
| `personal` | One per user, created on signup | None |
| `organization` | Shared team account | Linked to WorkOS Organization via `workos_org_id` |

Every account has members tracked in `account_members`. Personal accounts have a single `owner`. Organization accounts can have many members with different roles.

## System Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          ORGANIZATIONS ARCHITECTURE                      │
└──────────────────────────────────────────────────────────────────────────┘

  ┌──────────────┐       ┌──────────────┐       ┌──────────────────────┐
  │   Frontend   │       │   CLI        │       │   WorkOS             │
  │   (React)    │       │   (astro)    │       │   (Identity Provider)│
  └──────┬───────┘       └──────┬───────┘       └──────────┬───────────┘
         │                      │                          │
         │  Session Cookie      │  Bearer Token            │
         │  (AES-256-GCM)       │  (JWT via JWKS)          │
         ▼                      ▼                          │
  ┌────────────────────────────────────────────┐           │
  │              astro-server                  │           │
  │                                            │           │
  │  ┌────────────────────────────────────┐    │           │
  │  │          Middleware Stack           │    │           │
  │  │                                    │    │           │
  │  │  RequireAuth ──▶ ResolveAccount    │    │           │
  │  │       │              │             │    │           │
  │  │       ▼              ▼             │    │           │
  │  │  RequireAccountPermission          │    │           │
  │  │  (personal owner / org JWT)        │    │           │
  │  └────────────────────────────────────┘    │           │
  │                                            │           │
  │  ┌──────────────┐  ┌──────────────────┐    │           │
  │  │  org.Sync    │  │ org.Events       │    │           │
  │  │  (write path)│──│ Consumer         │◀───┼───────────┘
  │  │  Astro→WorkOS│  │ (read path)      │    │  Events API polling
  │  └──────────────┘  │ WorkOS→Astro     │    │
  │         │          └──────────────────┘    │
  │         ▼                    │              │
  │  ┌───────────────────────────────────┐     │
  │  │          PostgreSQL               │     │
  │  │  accounts · account_members       │     │
  │  │  workos_event_cursor              │     │
  │  └───────────────────────────────────┘     │
  └────────────────────────────────────────────┘
```

## Authentication Flow

Authentication is handled by WorkOS AuthKit. See [authentication-flow.md](authentication-flow.md) for the full OAuth and session management details.

What's relevant for organizations:

1. **Login/Callback** — After OAuth, the session JWT may carry an `OrganizationID`, `Role`, and `Permissions` if the user has an active org context.
2. **Login-time sync** — On every login and token refresh, `SyncMembershipsForUser` reconciles the user's WorkOS memberships into local `account_members` rows.
3. **Org context switching** — `POST /auth/switch-org` refreshes the JWT scoped to a specific WorkOS organization, giving the user role and permissions claims for that org.

```
┌──────────────────────────────────────────────────────────────────────────┐
│                       ORGANIZATION CONTEXT SWITCHING                      │
└──────────────────────────────────────────────────────────────────────────┘

  User has accounts: [personal, org-A, org-B]
  Currently scoped to: org-A

  POST /auth/switch-org { "organization_id": "<org-B workos_org_id>" }
       │
       ▼
  1. Unseal current session cookie
  2. Call WorkOS: refresh token scoped to org-B
  3. WorkOS returns new JWT with:
     - organization_id: org-B
     - role: "admin"
     - permissions: ["agents:read", "agents:write", "agents:deploy", "org:manage"]
  4. Seal new session → update cookie
  5. Return updated auth response with role + permissions

  Now scoped to: org-B (JWT carries org-B permissions)
```

## Permission Model

### Roles and Permissions

| Role | `agents:read` | `agents:write` | `agents:deploy` | `org:manage` | `org:admin` |
|------|:---:|:---:|:---:|:---:|:---:|
| **owner** | Y | Y | Y | Y | Y |
| **admin** | Y | Y | Y | Y | - |
| **member** | Y | - | Y | - | - |

Permission slugs map to actions:

| Permission | Grants |
|---|---|
| `agents:read` | View agents, versions, configs |
| `agents:write` | Push (register) agents, set visibility |
| `agents:deploy` | Deploy and undeploy agents |
| `org:manage` | Manage members and invitations |
| `org:admin` | Rename/delete org, billing |

### Authorization Flow

`RequireAccountPermission` middleware enforces permissions based on account type:

```
RequireAccountPermission(permission)
       │
       ├─ PERSONAL ACCOUNT
       │     Is caller the owner?
       │     ├─ Yes → ALLOW (all permissions implicit)
       │     └─ No  → DENY
       │
       └─ ORGANIZATION ACCOUNT
             Is JWT scoped to this org?
             (session.OrganizationID == account.WorkOSOrganizationID)
             ├─ No  → DENY ("use switch-org first")
             └─ Yes → Check session.Permissions array
                      ├─ Permission found → ALLOW
                      └─ Not found        → DENY
```

Organization accounts require the session JWT to be scoped to the target org via `POST /auth/switch-org`. The JWT carries the user's permissions for that org directly from WorkOS — no DB lookup needed. If the JWT is scoped to a different org, the request is rejected with 403.

### Route Protection

| Route | Permission | Description |
|---|---|---|
| `PUT /accounts/:account` | `org:admin` | Rename account |
| `GET/POST/PUT/DELETE .../members` | `org:manage` | Member CRUD |
| `GET/POST/DELETE .../invitations` | `org:manage` | Invitation CRUD |
| `POST /agents/:account/:name/register` | `agents:write` | Register agent build |
| `PUT /agents/:account/:name/visibility` | `agents:write` | Set public/private |

## Organization Account Lifecycle

When a user creates an organization account, a multi-step provisioning flow runs with compensating actions on failure:

```
POST /api/v1/accounts { "name": "my-org", "type": "organization" }

Step 1: Create local account
  └─ INSERT into accounts + account_members (creator as owner)
  └─ On failure → return error

Step 2: Create WorkOS Organization
  └─ WorkOS API: create org with external_id = account.id
  └─ On failure → DELETE local account (compensating action)

Step 3: Link WorkOS org to local account
  └─ UPDATE accounts SET workos_org_id = <workos_org_id>
  └─ On failure → DELETE WorkOS org + DELETE local account

Step 4: Create WorkOS membership for creator
  └─ WorkOS API: create membership (creator, role=owner)
  └─ On failure → non-fatal (local membership already exists)
  └─ On success → UPDATE local member with workos_membership_id
```

## Membership Sync Architecture

Memberships are kept in sync between Astro's `account_members` table and WorkOS through three mechanisms:

```
┌──────────────────────────────────────────────────────────────────────────┐
│                        MEMBERSHIP SYNC ARCHITECTURE                      │
└──────────────────────────────────────────────────────────────────────────┘

         WRITE PATH (Astro → WorkOS)             READ PATH (WorkOS → Astro)
         ──────────────────────────              ──────────────────────────

  Handler calls org.Sync method         org.EventsConsumer polls WorkOS
           │                                        │
           ▼                                        ▼
  1. Write to WorkOS first              1. List events since cursor
  2. On success → write locally         2. Process each event:
  3. On local failure → compensate         - membership.created → upsert
     (delete WorkOS membership)            - membership.updated → upsert
                                           - membership.deleted → remove
                                        3. Persist cursor for idempotency

                      LOGIN-TIME RECONCILIATION
                      ─────────────────────────

  On login / token refresh:
  1. List all active WorkOS memberships for user
  2. For each membership with a known local account:
     └─ Upsert into account_members
  3. Catches any events missed between polls
```

### Write Path: `org.Sync`

All member mutations flow through `org.Sync` to ensure WorkOS is updated first:

| Method | WorkOS Action | Local Action | Compensating Action |
|---|---|---|---|
| `AddMember` | Create membership | Insert `account_members` | Delete WorkOS membership |
| `ChangeMemberRole` | Update membership role | Update local role | (none needed) |
| `RemoveMember` | Delete membership | Delete local row | (none needed) |

**Safety guards** built into the write path:
- `ChangeMemberRole` prevents demoting the last owner
- `RemoveMember` prevents removing the last owner

### Read Path: `org.EventsConsumer`

A background goroutine polls the WorkOS Events API on a configurable interval:

- **Events processed**: `organization_membership.created`, `.updated`, `.deleted`
- **Cursor tracking**: A singleton `workos_event_cursor` table stores the last processed event ID
- **Idempotent**: Uses `INSERT ON CONFLICT` for both member upserts and cursor persistence
- **Fault tolerant**: A single event failure doesn't block processing of subsequent events

### Login-Time Reconciliation

`SyncMembershipsForUser` runs on every login and token refresh as a best-effort catch-up mechanism. It lists all active WorkOS memberships for the user and upserts them locally, ensuring any events missed between polls are recovered.

## Agent Visibility

Agent visibility replaces the previous semver publishing model. Agents are either `public` or `private` (default `private`).

| Visibility | Public catalog | Direct access | Validation warnings |
|---|---|---|---|
| `public` | Listed in `GET /api/v1/agents` | Anyone can view | Not shown to non-members |
| `private` | Not listed | Only account members | Shown to account members |

- `PUT /agents/:account/:name/visibility` toggles between `public` and `private` (requires `agents:write`)
- Non-members requesting a private agent get `404` (not `403`) to avoid revealing existence
- The `agents` table has a partial index on `visibility = 'public'` for catalog queries

## Database Schema

### Core Tables

```sql
-- Accounts (personal or organization)
accounts (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name          text UNIQUE NOT NULL,
  type          varchar(20) NOT NULL,        -- 'personal' | 'organization'
  created_at    timestamptz NOT NULL,
  updated_at    timestamptz NOT NULL
)

-- Links organization accounts to WorkOS
account_organizations (
  account_id    uuid PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  workos_org_id text NOT NULL UNIQUE
)

-- Account memberships
account_members (
  account_id           uuid REFERENCES accounts(id) ON DELETE CASCADE,
  user_id              text NOT NULL,
  role                 varchar(20) NOT NULL,  -- 'owner' | 'admin' | 'member'
  workos_membership_id text,                  -- NULL for personal accounts
  created_at           timestamptz NOT NULL,
  PRIMARY KEY (account_id, user_id)
)

-- Events consumer cursor (singleton)
workos_event_cursor (
  id         int PRIMARY KEY DEFAULT 1,
  cursor_id  text NOT NULL,
  updated_at timestamptz NOT NULL
)
```

### Key Indexes

- `account_organizations.workos_org_id` — unique index
- `account_members.workos_membership_id` — partial unique index (WHERE `workos_membership_id IS NOT NULL`)
- `agents.visibility` — partial index (WHERE `visibility = 'public'`)

## Internal Packages

### `internal/org`

| Component | Purpose |
|---|---|
| `Client` | Wraps WorkOS SDK for organizations, memberships, invitations, and roles |
| `Sync` | Write-path logic: Astro → WorkOS, with compensating actions |
| `EventsConsumer` | Background goroutine polling WorkOS Events API for membership changes |
| `types.go` | Organization, Membership, Invitation, Role domain types |

### `internal/account`

| Component | Purpose |
|---|---|
| `AccountStore` | PostgreSQL CRUD for accounts and members |
| `ValidateAccountName` | Name validation: 4-39 chars, lowercase, no reserved names |
| `types.go` | Account, AccountMember, AccountWithRole structs |

### `internal/middleware`

| Component | Purpose |
|---|---|
| `ResolveAccount` | Looks up `:account` URL param and sets it in context |
| `RequireAccountPermission` | Authorization: personal (owner check) or org (JWT permissions) |

## API Endpoints

| Method | Path | Auth | Permission | Description |
|---|---|---|---|---|
| `POST` | `/api/v1/accounts` | Required | - | Create account |
| `GET` | `/api/v1/accounts/:account` | Optional | - | Get account (public) |
| `PUT` | `/api/v1/accounts/:account` | Required | `org:admin` | Rename account |
| `GET` | `.../:account/members` | Required | `org:manage` | List members |
| `POST` | `.../:account/members` | Required | `org:manage` | Add member |
| `PUT` | `.../:account/members/:user_id` | Required | `org:manage` | Change role |
| `DELETE` | `.../:account/members/:user_id` | Required | `org:manage` | Remove member |
| `GET` | `.../:account/invitations` | Required | `org:manage` | List invitations |
| `POST` | `.../:account/invitations` | Required | `org:manage` | Send invitation |
| `DELETE` | `.../:account/invitations/:id` | Required | `org:manage` | Revoke invitation |
| `PUT` | `/agents/:account/:name/visibility` | Required | `agents:write` | Set visibility |
| `POST` | `/auth/switch-org` | Session | - | Switch org context |
