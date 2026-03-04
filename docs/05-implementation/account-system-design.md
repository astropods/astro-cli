# Account System Design

Introduces GitHub-style accounts to establish globally unique namespaces for agents and all scoped resources.

## Core Concept

An **account** is a globally unique, human-readable name that owns agents and scopes all resources. Two types:

- **Personal account** — 1:1 with a WorkOS user. Created when user claims a username via the web dashboard. Every user must have one before they can publish.
- **Organization account** — shared namespace for teams. Users can create and join orgs. Multiple members with role-based access.

Agent names are scoped to accounts: `saswat/engineering-assistant`, `postman/engineering-assistant` — both can coexist.

## Account Name Rules

- 2-39 characters
- Lowercase alphanumeric and hyphens only
- Must start with a letter
- Must not end with a hyphen
- No consecutive hyphens
- Globally unique across personal and org accounts
- Reserved names: `admin`, `api`, `www`, `app`, `system`, `astro`, `registry`, etc.

## Data Model

```sql
CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(39) UNIQUE NOT NULL,         -- the handle, globally unique
    type VARCHAR(20) NOT NULL,                -- 'personal' or 'organization'
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
-- Display name, avatar, etc. come from WorkOS user/org profile at read time. Not stored here.

CREATE TABLE account_members (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_id VARCHAR(255) NOT NULL,            -- WorkOS user ID
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, user_id)
);

-- Index for "given a user, find their accounts"
CREATE INDEX idx_account_members_user ON account_members(user_id);

-- Roles are managed in WorkOS, not stored locally.
-- Personal account: exactly one member (the creator)
-- Org account: members tracked locally for fast lookup, roles from WorkOS JWT
```

Account name rules are a superset of k8s namespace rules, so the account name can be used directly as the k8s namespace with no sanitization.

## Account name vs account ID

Account names are mutable — users can rename at any time. This means anything persisted or used as infrastructure identity must use the stable **account ID**, not the name.

**Use account ID (stable, survives renames):**
- K8s namespace
- K8s resource labels
- ECR repository naming — `tenant-{account_id}/...`

**Use account name (human-readable, breaking on rename):**
- Registry image paths — `registry.host/saswat/my-agent:tag`
- API route resolution — `/api/v1/agents/saswat/my-agent`
- Stored image refs in build/spec records
- Dashboard UI, CLI output

Renaming an account is a destructive action. Existing registry images and active deployments reference the old name. The rename flow must warn the user and require re-publishing.

## How accounts scope existing resources

| Resource | Before | After |
|----------|--------|-------|
| Agent identity | `my-agent` (global, can collide) | `saswat/my-agent` (account-scoped) |
| Registry image path | `registry.host/{workos_user_id}/my-agent:tag` | `registry.host/{account_name}/my-agent:tag` |
| K8s namespace | `sanitizeNamespace(workos_user_id)` → opaque UUID | `a-{account_id}` (stable) |
| API paths | `/api/v1/agents/:name` | `/api/v1/agents/:account/:name` |
| Agent index PK | `(name, version)` | `(account_id, name, ...)` |

## Registration Flow

Account claiming happens on the **web dashboard only**. CLI reads account info from the server after login.

### First-time user flow

1. User signs up via WorkOS (existing)
2. WorkOS redirects to dashboard. `GET /auth/me` returns `accounts: []`.
3. Dashboard detects empty accounts and shows onboarding screen: "Choose your username". Validates format client-side (length, charset, pattern). Checks uniqueness + reserved names server-side via `GET /api/v1/accounts/check/:name` with debounce.
4. Submit calls `POST /api/v1/accounts` with `type: personal`. Server creates account + account_member.
5. Dashboard proceeds to main app. All subsequent pages gate on `accounts.length > 0`.

CLI equivalent: `ast login` calls `GET /auth/me`. If `accounts: []`, prints:
```
Login successful. Visit https://app.astropods.ai to choose your username before publishing.
```

### Org creation flow

Deferred. The data model supports orgs (account type + members table) but no UI/API for creating or managing orgs in v1.

### CLI flow

1. `ast login` — authenticates with WorkOS (unchanged)
2. After successful auth, CLI calls `GET /auth/me` to fetch user profile + accounts
3. Stores account info in credentials profile:
   ```json
   {
     "user": { "id": "user_01K1...", "email": "...", "username": "saswat" },
     "accounts": [
       { "name": "saswat", "type": "personal" },
       { "name": "postman", "type": "organization" }
     ],
     "current_account": "saswat"
   }
   ```
4. `ast whoami` — shows username + available accounts
5. If user has no personal account yet, CLI prints:
   ```
   Login successful. Visit https://app.astropods.ai to set up your account before publishing.
   ```

## API

### Account endpoints

```
GET    /auth/me                                      — current user profile + accounts (cookie or Bearer)
POST   /api/v1/accounts                              — create account (personal or org)
GET    /api/v1/accounts/:account                     — get account info (display name/avatar from WorkOS)
GET    /api/v1/accounts/check/:name                  — check account name availability
PUT    /api/v1/accounts/:account                     — rename account (destructive)
GET    /api/v1/accounts/:account/members             — list members (orgs only, deferred)
POST   /api/v1/accounts/:account/members             — invite member (deferred)
DELETE /api/v1/accounts/:account/members/:user_id    — remove member (deferred)
```

### How account changes existing agent endpoints

All agent endpoints gain the account prefix. Version handling is out of scope for this doc.

```
Before:  GET  /api/v1/agents/:name
After:   GET  /api/v1/agents/:account/:name

Before:  POST /api/v1/agents/register
After:   POST /api/v1/agents/:account/:name/register

Before:  GET  /api/v1/agents/:name/:version/config
After:   GET  /api/v1/agents/:account/:name/config
```

### How account changes existing deployment endpoints

```
Before:  POST /api/v1/deploy               (namespace derived from user ID)
After:   POST /api/v1/deploy               (namespace derived from account in request body)

Before:  GET  /api/v1/deployments           (filtered by user namespace)
After:   GET  /api/v1/deployments           (filtered by account membership)
```

### Authorization rules

| Action | Required role |
|--------|-------------|
| Read public agent | Any (unauthenticated OK) |
| Publish agent to account | `owner` or `admin` of account |
| Deploy agent from account | `owner`, `admin`, or `member` of account |
| Create org | Any authenticated user with a personal account |
| Manage org members | `owner` or `admin` of org |
| Delete account | `owner` only |

## Registry Changes

The registry connects to the shared Postgres database to resolve account names and check membership. No separate namespace API endpoint — the CLI reads account info from the stored profile after login.

### Image paths

```
Before:  registry.host/user_01K1VMRDRQ94MV98D9ANFVT7H2/my-agent:tag
After:   registry.host/saswat/my-agent:tag
```

### ECR backend paths

The registry translates human-readable account names to stable ECR paths using account IDs. This survives account renames.

```
Client pushes to:   registry.host/saswat/my-agent:tag
ECR stores as:      tenant-{account_uuid}/my-agent:tag
```

`addTenantPrefix()` looks up the account by name via `MembershipChecker.GetAccountByName()` and rewrites the path. `stripTenantPrefix()` reverses this for Location header rewriting.

### Authorization

`validateNamespaceAccess()` checks account membership via `MembershipChecker.IsMember(accountName, userID)`. Read operations (GET/HEAD) are allowed for any authenticated user. Write operations (POST/PUT/PATCH) require membership in the target account.

### Registry architecture

- DB connection initialized in `main.go`, passed as `MembershipChecker` to proxy config
- No `/api/namespace` endpoint — CLI reads account info from stored profile (populated during `ast login`)
- Location headers from ECR are rewritten to strip tenant prefix before returning to client

## K8s Namespace Derivation

Account names are mutable (users can rename), so the k8s namespace uses the account UUID instead:

```
Before:  sanitizeNamespace(user.ID) → "user-01k1vmrdrq94mv98d9anfvt7h2"
After:   account ID (e.g. "a-3f2a1b4c") — stable across renames
```

The namespace is derived once at account creation and never changes. Format: `a-{short_uuid}` (prefix ensures valid k8s name starting with a letter, short enough to stay under 63 char limit).

## Server Implementation

### Account store

New package: `apps/astro-server/internal/account/`

```go
type Store struct { db *sql.DB }

func (s *Store) CreateAccount(name, accountType, displayName string) (*Account, error)
func (s *Store) GetAccount(name string) (*Account, error)
func (s *Store) GetAccountsForUser(userID string) ([]AccountMembership, error)
func (s *Store) AddMember(accountID, userID, workosMembershipID string) error
func (s *Store) RemoveMember(accountID, userID string) error
func (s *Store) GetMembers(accountID string) ([]Member, error)
func (s *Store) IsMember(accountID, userID string) (bool, error)
```

### Auth middleware update

After JWT validation, attach account memberships to request context:

```go
accounts := accountStore.GetAccountsForUser(user.ID)
ctx = context.WithValue(ctx, "accounts", accounts)
```

Route handlers check account membership for authorization.

## Files to Create/Modify

| File | Action |
|------|--------|
| `apps/astro-server/internal/account/store.go` | New — account CRUD |
| `apps/astro-server/internal/account/types.go` | New — Account, Member types |
| `apps/astro-server/internal/account/validate.go` | New — account name validation + reserved names |
| `apps/astro-server/migrations/` | New — accounts, account_members tables |
| `apps/astro-server/handlers/accounts.go` | New — account + member API handlers |
| `apps/astro-server/handlers/agents.go` | Modify — add `:account` to routes |
| `apps/astro-server/handlers/deploy.go` | Modify — namespace from account |
| `apps/astro-server/main.go` | Modify — new routes, inject account store |
| `apps/astro-server/internal/middleware/auth.go` | Modify — attach account memberships to context |
| `apps/astro-server/internal/agentindex/index.go` | Modify — account-scoped queries |
| `apps/astro-registry/internal/account/membership.go` | New — lightweight MembershipChecker (GetAccountByName, IsMember, GetAccountsForUser) |
| `apps/astro-registry/internal/config/config.go` | Modify — add Database.URL config |
| `apps/astro-registry/handlers/registry_proxy.go` | Modify — account-based namespace validation, tenant-prefixed ECR paths |
| `apps/astro-registry/main.go` | Modify — init DB + MembershipChecker, remove GET /api/namespace |
| `apps/astro-cli/cmd/login.go` | Modify — fetch accounts after auth |
| `apps/astro-cli/cmd/whoami.go` | Modify — show accounts |
| `apps/astro-cli/cmd/publish.go` | Modify — account-scoped image names |
| `apps/astro-cli/internal/auth/storage.go` | Modify — store account info in profile |
| `apps/astro-client/src/pages/` | Modify — onboarding flow for account creation |
| `apps/astro-client/src/api/` | Modify — account-prefixed API calls |
