# GitHub Webhook Consolidation

**Status**: Proposed
**Date**: 2026-04-26

When two Astro accounts connect to the same GitHub repo, each creates its own webhook. GitHub fires both per push — double the API traffic, duplicate entries in the repo's webhook delivery log, and confusing redundancy for repo admins. This spec consolidates webhook ownership into a global `github_webhooks` table so the invariant ("one webhook per repo") is enforced by the schema rather than scattered application-level logic.

---

## 1. Problem

`webhook_id` and `webhook_secret` live on `github_connections`, which is scoped per account. When Account A and Account B both connect `owner/repo`, the link handler checks for an existing webhook within each account's connections, finds none, and calls the GitHub API twice. Two webhooks are created pointing at the same Astro endpoint. GitHub fires both per push.

The current dedup logic is account-scoped — it prevents duplicate webhooks within a single account but not across accounts.

---

## 2. Goals

1. One webhook per GitHub base repo (`owner/repo`) regardless of how many Astro accounts are connected.
2. A push to `owner/repo` triggers builds in every account that has agents connected to that repo and branch.
3. Connect and disconnect are race-safe under concurrent requests across multiple server instances.

## 3. Non-Goals

1. GitHub App-based auth (scope: OAuth tokens only).

---

## 4. Data Model

### 4.1 New table: `github_webhooks`

```sql
CREATE TABLE github_webhooks (
    repo_base      varchar PRIMARY KEY,
    webhook_id     bigint  NOT NULL,
    webhook_secret varchar NOT NULL,
    created_at     timestamp DEFAULT now()
);
```

`repo_base` is the primary key. The PRIMARY KEY constraint makes it impossible to represent two webhooks for the same repo at the schema level.

### 4.2 Changes to `github_connections`

Drop `webhook_id` and `webhook_secret`. A connection record expresses intent — "this agent wants builds from this repo+branch" — and nothing more. Webhook ownership moves entirely to `github_webhooks`.

### 4.3 New store: `internal/githubwebhook/store.go`

```go
func (s *Store) Get(ctx context.Context, repoBase string) (*Webhook, error)
func (s *Store) Insert(ctx context.Context, repoBase string, webhookID int64, secret string) error
func (s *Store) DeleteIfNoConnections(ctx context.Context, repoBase string) (webhookID int64, deleted bool, err error)
```

`DeleteIfNoConnections` atomically deletes the `github_webhooks` row only if no `github_connections` rows reference that `repo_base`, returning the `webhook_id` so the caller can delete it from GitHub:

```sql
DELETE FROM github_webhooks
WHERE repo_base = $1
AND NOT EXISTS (
    SELECT 1 FROM github_connections
    WHERE repo_full_name = $1 OR repo_full_name LIKE $1 || '/%'
)
RETURNING webhook_id
```

---

## 5. Handler Changes

### 5.1 `GitHubLink`

The connection record is saved first (existing behavior), then `webhookStore.Get(repoBase)` replaces `GetByRepoBaseForAccount(accountID, repoBase)` — global, no account scope. If a webhook exists, no GitHub API call is made. If not, the webhook is created and inserted into `github_webhooks`. On INSERT conflict, read the existing row and delete the orphaned webhook just created from GitHub.

### 5.2 `GitHubWebhook` (inbound push handler)

Replace `ListByRepoAndBranchForAccount(accountID, repo, branch)` with `ListByRepoAndBranch(repo, branch)`. One verified push triggers builds for every account's agents connected to that repo and branch. The per-account scoping was a consequence of per-account secrets; with a single global secret the scoping is no longer meaningful.

### 5.3 `GitHubDisconnect`, `GitHubAccountDisconnect`, `ArchiveAgent`

Replace `CountByRepoBaseForAccount(accountID, repoBase)` + conditional delete with `webhookStore.DeleteIfNoConnections(repoBase)`. If it returns `deleted = true`, call `gh.DeleteWebhook(webhookID)`.

---

## 6. Migration

The repo uses Atlas with a declarative schema — changes are made to `sql/astro-server/schema.sql` and Atlas generates the DDL on apply. Atlas only handles structural changes, not data migrations, so the backfill is a separate step.

**Schema changes (`sql/astro-server/schema.sql`):**
- Add `github_webhooks` table
- Drop `webhook_id` and `webhook_secret` from `github_connections`

**Backfill (River job, run before schema apply drops the columns):**

Populate `github_webhooks` from `github_connections` — for each distinct `repo_base`, pick the row with the earliest `created_at` where `webhook_id != 0`:

```sql
INSERT INTO github_webhooks (repo_base, webhook_id, webhook_secret, created_at)
SELECT DISTINCT ON (repo_base)
    split_part(repo_full_name, '/', 1) || '/' || split_part(repo_full_name, '/', 2),
    webhook_id, webhook_secret, created_at
FROM github_connections
WHERE webhook_id != 0
ORDER BY repo_base, created_at ASC;
```

Connections with `webhook_id = 0` (manual-build-only agents) have no webhook to migrate. Duplicate webhooks from multiple accounts for the same repo become orphaned on GitHub and can be removed manually.

---

## 7. Files

| File | Change |
|------|--------|
| `sql/astro-server/schema.sql` | Add `github_webhooks` table; drop `webhook_id`, `webhook_secret` from `github_connections` |
| `apps/astro-server/internal/githubwebhook/store.go` | **New** — `Get`, `Insert`, `DeleteIfNoConnections` |
| `apps/astro-server/internal/githubconnection/store.go` | Remove `WebhookID`, `WebhookSecret` from `Connection`; remove `GetByRepoBase`, `GetByRepoBaseForAccount`, `CountByRepoBaseForAccount`; add global `ListByRepoAndBranch` |
| `apps/astro-server/internal/riverqueue/` | **New** backfill job to populate `github_webhooks` before column drop |
| `apps/astro-server/handlers/github.go` | Update `GitHubLink`, `GitHubWebhook`, `GitHubDisconnect`, `GitHubAccountDisconnect` |
| `apps/astro-server/handlers/agents.go` | Update `ArchiveAgent` goroutine |
| `apps/astro-server/main.go` | Wire `githubwebhook.Store` into handlers |

---

## 8. Race Conditions

### 8.1 Race A — two accounts connect the same repo simultaneously

Both check `github_webhooks` → not found → both call GitHub API → both try INSERT. The PRIMARY KEY constraint means only one INSERT succeeds. The loser catches the conflict error, reads the winner's row, deletes its orphaned webhook from GitHub, and continues using the winner's secret.

### 8.2 Race B — disconnect races with a concurrent connect

The existing code already saves the connection record before checking for or creating the webhook. This ordering makes Race B safe:

- If disconnect runs after connect's connection INSERT: `DeleteIfNoConnections` sees count ≥ 1 and skips deletion.
- If disconnect runs before connect's connection INSERT: `DeleteIfNoConnections` deletes the webhook row. Connect then finds `github_webhooks` empty and creates a fresh webhook.

---

## 9. Key Decisions

**`repo_base` as primary key** — there is no scenario where two rows for the same `repo_base` should exist. Making it the PK means the constraint is definitional, not supplemental.

**Atomic `DeleteIfNoConnections`** — combining the count check and the delete into a single SQL statement eliminates the window between them. No advisory locks are needed because the existing connection-first ordering in `GitHubLink` already prevents disconnect from racing ahead of a concurrent connect.

**Global fan-out is safe with a single secret** — the per-account scoping was a consequence of per-account secrets, not a security measure. With one secret in `github_webhooks`, HMAC verification still proves the push came from GitHub. Triggering builds in all connected accounts is the intended behavior, not a cross-account leak.

**Orphaned webhook cleanup is out-of-band** — the migration backfills `github_webhooks` from the first account's webhook per repo but does not call GitHub's API to delete duplicates. A migration should not make external API calls.
