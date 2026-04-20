# Monorepo GitHub Connection Spec

**Version:** 1.0
**Date:** 2026-04-20
**Status:** Draft

## Abstract

This spec adds `sub_path` support to the GitHub connection flow, allowing multiple blueprints to connect to the same GitHub repository at different subdirectories. The `sub_path` controls where `astropods.yml` is looked up and is the root for paths declared within the spec. No two blueprints may connect to the same `(repo, sub_path)` pair within an account. Webhook state is extracted into a dedicated `github_webhooks` table to enforce deduplication at the DB level.

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 1. Problem

The current GitHub connection model enforces a `UNIQUE (account_id, repo_full_name)` constraint — one blueprint per repository per account. This prevents teams using monorepos from connecting multiple blueprints to the same repository. Each blueprint must live in its own dedicated repository today.

---

## 2. Goals

1. **Monorepo support** — multiple blueprints MUST be connectable to the same GitHub repository within an account.
2. **Subfolder scoping** — each connection MUST declare a `sub_path` specifying the directory containing `astropods.yml`. An empty `sub_path` means the repository root.
3. **Uniqueness at `(repo, sub_path)`** — no two blueprints within an account MAY share the same `(repo_full_name, sub_path)` pair.
4. **Webhook deduplication** — a single GitHub webhook MUST serve all blueprints connected to the same repository. Uniqueness is enforced at the DB level.
5. **Independent builds** — each blueprint's build MUST be triggered and tracked independently, even when sharing a repository.
6. **Backward compatibility** — existing connections (all at `sub_path = ""`) MUST continue to work without migration.

## 3. Non-Goals

1. **Frontend changes** — repo selection UI, subfolder input, and connected-repo display are out of scope for this spec.
2. **Additional watch paths** — configuring extra paths outside `sub_path` to trigger a build (e.g. shared libraries) is out of scope.

---

## 4. Data Model

### 4.1 New `github_webhooks` table

Owns the GitHub webhook for a repository. One row per repository, shared across all blueprints connected to that repo.

```sql
CREATE TABLE public.github_webhooks (
    repo_full_name varchar  NOT NULL,
    webhook_id     bigint   NOT NULL,
    webhook_secret varchar  NOT NULL,
    created_at     timestamp NOT NULL DEFAULT now(),
    CONSTRAINT github_webhooks_pkey PRIMARY KEY (repo_full_name)
);
```

The `PRIMARY KEY (repo_full_name)` enforces at the DB level that only one webhook can exist per repository. Concurrent link requests for the same repo resolve via `INSERT ... ON CONFLICT DO NOTHING` — the first insert wins and subsequent requests reuse the existing row.

### 4.2 `github_connections` schema changes

Add `sub_path`:

```sql
ALTER TABLE public.github_connections
  ADD COLUMN sub_path varchar NOT NULL DEFAULT '';
```

Remove `webhook_id` and `webhook_secret` — these move to `github_webhooks`:

```sql
ALTER TABLE public.github_connections
  DROP COLUMN webhook_id,
  DROP COLUMN webhook_secret;
```

Drop the existing repo uniqueness index and replace with a composite that includes `sub_path`:

```sql
DROP INDEX idx_github_connections_account_repo;

CREATE UNIQUE INDEX idx_github_connections_account_repo_subpath
  ON public.github_connections (account_id, repo_full_name, sub_path);
```

The existing `UNIQUE (account_id, agent_name)` constraint is unchanged — one blueprint still maps to exactly one `(repo, sub_path)`.

`sub_path` is a relative Unix path (e.g. `services/my-agent`). Empty string means the repository root. Leading and trailing slashes are stripped on write.

### 4.3 Struct changes

`Connection` struct: add `SubPath string` after `Branch`; remove `WebhookID` and `WebhookSecret`.

New `Webhook` struct:

```go
type Webhook struct {
    RepoFullName  string
    WebhookID     int64
    WebhookSecret string
    CreatedAt     time.Time
}
```

---

## 5. Spec Lookup and Path Resolution

When `sub_path` is non-empty, `astropods.yml` is fetched from `{sub_path}/astropods.yml` and `AGENT.md` is fetched from `{sub_path}/AGENT.md`.

Paths declared within the spec (`build.Context`, `build.Dockerfile`) are treated as relative to `sub_path`. The build system prepends `sub_path` to these values before executing the build:

- `build.Context = "."` → effective context is `{sub_path}`
- `build.Context = "subdir"` → effective context is `{sub_path}/subdir`
- `build.Dockerfile = "Dockerfile"` → effective dockerfile is `{sub_path}/Dockerfile`

When `sub_path` is empty, behavior is unchanged — paths resolve relative to the repository root.

---

## 6. Linking

### 6.1 Request

`POST /api/v1/agents/:account/:name/github/link` accepts an optional `sub_path` field:

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `repo_full_name` | string | yes | `owner/repo` |
| `branch` | string | no | defaults to `main` |
| `sub_path` | string | no | defaults to `""` (repo root) |

### 6.2 Validation

`sub_path` MUST match `^[a-zA-Z0-9._/-]*$` with no leading slash and no `..` path segments. Invalid values return HTTP 400.

### 6.3 Conflict check

Before upserting, the handler MUST check whether any other blueprint in the account already holds `(repo_full_name, sub_path)`. If a conflict exists, it MUST return HTTP 409: `"repo %q path %q already connected to agent %q"`.

### 6.4 Webhook deduplication

The handler MUST use the following sequence:

1. Call `GetWebhook(repo_full_name)`. If a row exists, reuse it — no GitHub API call needed.
2. If no row exists, generate a `webhook_secret`, call the GitHub API to create the webhook, and receive back a `webhook_id`.
3. Attempt:
   ```sql
   INSERT INTO github_webhooks (repo_full_name, webhook_id, webhook_secret)
   VALUES ($1, $2, $3)
   ON CONFLICT (repo_full_name) DO NOTHING
   ```
4. If the insert conflicted (a concurrent request won the race), delete the just-created GitHub webhook via the API and call `GetWebhook` to retrieve the winning row.

---

## 7. Webhook Fan-out

On receiving a push event, the webhook handler MUST:

1. Look up the webhook secret via `github_webhooks WHERE repo_full_name = $1` to verify the HMAC signature.
2. Query `github_connections WHERE repo_full_name = $1 AND branch = $2` to retrieve all blueprints connected to that repo and branch.
3. For each connection, apply path filtering (Section 7.1), then independently: create a build record, cancel superseded builds, and enqueue a River job.

Each blueprint's build is fully independent with its own build record and job.

### 7.1 Path Filtering

Before enqueuing a build for a connection, the handler MUST check whether the push touched any files under that connection's `sub_path`.

The push payload includes the union of `added`, `removed`, and `modified` file paths across all commits in the push. A connection is eligible for a build if at least one changed file path starts with `{sub_path}/`.

Connections with `sub_path = ""` (repo root) are always eligible — every push to the branch triggers a build.

If no changed files fall under `sub_path`, the connection is skipped and no build record is created.

Path filtering only applies to webhook-triggered builds. Manual rebuilds bypass path filtering entirely — they always enqueue a build for the specific blueprint the user triggered, regardless of what files changed.

---

## 8. Disconnect

On disconnect, the handler MUST delete the connection row, then call `CountByRepo(repo_full_name)` to check whether any remaining connections exist for that repository across all branches. If the count is zero, delete the `github_webhooks` row and call the GitHub API to remove the webhook.

`CountByRepo` MUST be added to the store: `SELECT COUNT(*) FROM github_connections WHERE repo_full_name = $1`.

---

## 9. AGENT.md Cache Key

`GitHubStatus` caches the draft `AGENT.md` content in Redis. The cache key MUST include `sub_path` to prevent two blueprints on the same repo and branch at different sub_paths from sharing a cache entry:

```
astro:github:agent-md:{repo}:{branch}:{sub_path}
```

When `sub_path` is empty the key is `astro:github:agent-md:{repo}:{branch}:` which is consistent with existing keys for root connections after migration.

---

## 10. Status Response

`GET /api/v1/agents/:account/:name/github` includes `sub_path` in the response:

| Field | Type | Notes |
|-------|------|-------|
| `sub_path` | string | omitted when empty |

---

## 11. Account Connections

`GET /api/v1/accounts/:account/github/connections` includes `sub_path` in each connection entry.

---

## 12. Scan

`GET /api/v1/accounts/:account/github/scan` accepts an optional `sub_path` query parameter. When provided, it scans `{sub_path}/astropods.yml` instead of `astropods.yml`.

---

## 13. Files

| File | Change |
|------|--------|
| `sql/astro-server/schema.sql` | Add `github_webhooks` table; add `sub_path`, drop `webhook_id`/`webhook_secret` from `github_connections`; replace uniqueness index |
| `apps/astro-server/internal/githubconnection/store.go` | **New** `Webhook` struct + `UpsertWebhook`, `GetWebhook`, `DeleteWebhook`, `CountByRepo` methods; **update** `Upsert`, `Get`, `GetByID`, `ListByAccount` to add `SubPath` and remove `WebhookID`/`WebhookSecret`; **rename** `GetByRepoForAccount` → `GetByRepoAndSubPathForAccount`; **replace** `GetByRepo` → `ListByRepoAndBranch(repoFullName, branch string)` |
| `apps/astro-server/internal/githubconnection/store_test.go` | Update existing tests for struct changes; add `TestStore_UpsertWebhook`, `TestStore_GetWebhook`, `TestStore_DeleteWebhook`, `TestStore_CountByRepo`, `TestStore_ListByRepoAndBranch`, `TestStore_GetByRepoAndSubPathForAccount` |
| `apps/astro-server/handlers/github.go` | `sub_path` in link request; updated conflict check; webhook dedup via `INSERT ON CONFLICT`; fan-out with path filtering; `sub_path` in status response; conditional webhook delete on disconnect; `sub_path` in account connections; scan sub_path param |
| `apps/astro-server/handlers/github_test.go` | Tests for all handler behavior changes |
| `apps/astro-server/internal/githubbuild/fetch.go` | `FetchAstroSpec` accepts `subPath`; prefixes lookup path when non-empty |
| `apps/astro-server/internal/githubbuild/fetch_test.go` | `TestFetchAstroSpec_SubPath` |
| `apps/astro-server/internal/githubbuild/builder.go` | `RunJob` accepts `subPath`; prepends it to `build.Context` and `build.Dockerfile` |
| `apps/astro-server/internal/githubbuild/builder_test.go` | Tests for context and dockerfile path prefixing |
| `apps/astro-server/internal/riverqueue/github_build.go` | Pass `conn.SubPath` to `FetchAstroSpec` and `RunJob` |
| `apps/astro-server/cmd/backfill-github-webhooks/main.go` | **New** — one-off script to copy `webhook_id`/`webhook_secret` from `github_connections` into `github_webhooks`; idempotent; supports `DRY_RUN=true` |

---

## 14. Migration Plan

Schema changes are applied via the `sql-migrate.yml` GitHub Action (Actions → "SQL Migrate (Prod)" → Run workflow). The workflow shows a diff, requires manual approval via the `gate` environment, applies, then posts to `#astro-ops`.

Because this change moves data out of `github_connections` before dropping columns, it MUST be split across two separate schema applies with a data backfill in between.

### Phase 1 — Additive changes only

Update `schema.sql`:
- Add `github_webhooks` table
- Add `sub_path varchar NOT NULL DEFAULT ''` to `github_connections`
- Drop `idx_github_connections_account_repo`, add `idx_github_connections_account_repo_subpath`

Do NOT remove `webhook_id` or `webhook_secret` yet. Trigger `sql-migrate.yml`.

### Phase 2 — Backfill webhook data

Run the one-off backfill script (following the pattern of `apps/astro-server/cmd/backfill-avatars/`):

```
DATABASE_URL=postgres://... go run ./cmd/backfill-github-webhooks
```

The script copies `webhook_id` and `webhook_secret` from `github_connections` into `github_webhooks` for all rows where `webhook_id IS NOT NULL`. It is idempotent (`ON CONFLICT DO NOTHING`) and supports `DRY_RUN=true`.

Verify the row count in `github_webhooks` matches the number of distinct repos in `github_connections` before proceeding.

### Phase 3 — Drop migrated columns

Update `schema.sql`:
- Remove `webhook_id` and `webhook_secret` from `github_connections`

Trigger `sql-migrate.yml`.

---

## 15. Implementation Order

1. Phase 1 schema migration — run `sql-migrate.yml`.
2. Store — `Webhook` struct and methods; `SubPath` on `Connection`; `CountByRepo`; updated queries.
3. Handlers — link conflict check, webhook dedup, fan-out with path filtering, disconnect guard, status/connections/scan; AGENT.md cache key.
4. Build — `FetchAstroSpec` sub_path lookup; `RunJob` path prefixing; River worker pass-through.
5. Deploy code to production.
6. Phase 2 backfill script — run `cmd/backfill-github-webhooks` against production.
7. Phase 3 schema migration — run `sql-migrate.yml` to drop `webhook_id`/`webhook_secret`.
