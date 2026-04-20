# Monorepo GitHub Connection Spec

**Version:** 1.0
**Date:** 2026-04-20
**Status:** Draft

## Abstract

This spec extends `repo_full_name` in the GitHub connection flow to support an optional subpath, allowing multiple blueprints to connect to the same GitHub repository at different subdirectories. `repo_full_name` stores either `owner/repo` (repository root) or `owner/repo/sub/path` (subdirectory). GitHub repo names are always exactly two path segments, so the boundary is unambiguous. No schema changes are required — the existing `UNIQUE (account_id, repo_full_name)` constraint already enforces the right uniqueness. `webhook_id` and `webhook_secret` stay on `github_connections`; webhook dedup is managed in application code.

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 1. Problem

The current GitHub connection model enforces a `UNIQUE (account_id, repo_full_name)` constraint — one blueprint per repository per account. This prevents teams using monorepos from connecting multiple blueprints to the same repository. Each blueprint must live in its own dedicated repository today.

---

## 2. Goals

1. **Monorepo support** — multiple blueprints MUST be connectable to the same GitHub repository within an account.
2. **Subfolder scoping** — each connection MUST declare where `astropods.yml` lives within the repository. No declaration means the repository root.
3. **Uniqueness at `(repo, subpath)`** — no two blueprints within an account MAY share the same `(repo_full_name, subpath)` pair.
4. **Webhook deduplication** — a single GitHub webhook MUST serve all blueprints connected to the same repository. Dedup is managed in application code.
5. **Independent builds** — each blueprint's build MUST be triggered and tracked independently, even when sharing a repository.
6. **Backward compatibility** — existing connections (all at the repository root) MUST continue to work without migration.

## 3. Non-Goals

1. **Frontend changes** — repo selection UI, subfolder input, and connected-repo display are out of scope for this spec.
2. **Additional watch paths** — configuring extra paths outside the subpath to trigger a build (e.g. shared libraries) is out of scope.

---

## 4. Data Model

### 4.1 `repo_full_name` encoding

`repo_full_name` in `github_connections` now stores one of:

- `owner/repo` — connection at the repository root (backward-compatible, existing behavior)
- `owner/repo/sub/path` — connection at a subdirectory

Two derived values are used throughout:

- **`repoBase`** — the first two `/`-joined segments (`owner/repo`). Used for GitHub API calls, git clone, and webhook operations.
- **`repoSubPath`** — everything after the third `/`, or empty string when connecting at root. Used for file path prefixing.

No schema changes are required. The existing `UNIQUE (account_id, repo_full_name)` constraint enforces uniqueness at the `(repo, subpath)` level, and the existing index on `repo_full_name` supports webhook fan-out lookups.

---

## 5. Spec Lookup and Path Resolution

When `repoSubPath(conn.RepoFullName)` is non-empty, `astropods.yml` is fetched from `{subPath}/astropods.yml` and `AGENT.md` is fetched from `{subPath}/AGENT.md`.

Paths declared within the spec (`build.Context`, `build.Dockerfile`) are treated as relative to `subPath`. The build system prepends `subPath` to these values before executing the build:

- `build.Context = "."` → effective context is `{subPath}`
- `build.Context = "subdir"` → effective context is `{subPath}/subdir`
- `build.Dockerfile = "Dockerfile"` → effective dockerfile is `{subPath}/Dockerfile`

When `subPath` is empty, behavior is unchanged — paths resolve relative to the repository root.

---

## 6. Linking

### 6.1 Request

`POST /api/v1/agents/:account/:name/github/link` accepts `repo_full_name` as either `owner/repo` or `owner/repo/sub/path`:

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `repo_full_name` | string | yes | `owner/repo` or `owner/repo/sub/path` |
| `branch` | string | no | defaults to `main` |

### 6.2 Validation

`repo_full_name` MUST have at least two segments. Segments beyond the second MUST each match `^[a-zA-Z0-9._-]+$` with no `..` components. Leading and trailing slashes are stripped on write. Invalid values return HTTP 400.

### 6.3 Conflict check

The handler attempts the upsert. If the DB returns a unique constraint violation on `(account_id, repo_full_name)`, the handler MUST query for the existing connection by `(account_id, repo_full_name)` to retrieve the conflicting agent name, then return HTTP 409: `"repo %q already connected to agent %q"`.

### 6.4 Webhook dedup

The handler MUST use the following sequence:

1. Call `GetByRepoBase(repoBase(repo_full_name))` to find any existing connection to the same base repository (across all accounts).
2. If a row exists, copy its `webhook_id` and `webhook_secret` to the new connection. No GitHub API call is needed.
3. If no row exists, generate a `webhook_secret`, call the GitHub API to create the webhook, and store the returned `webhook_id` and `webhook_secret` on the new connection row.

---

## 7. Webhook Fan-out

On receiving a push event, the webhook handler MUST:

1. Look up `webhook_secret` via `GetByRepoBase(payload.Repository.FullName)` to verify the HMAC signature.
2. Query all connections for that repo and branch:
   ```sql
   WHERE (repo_full_name = $1 OR repo_full_name LIKE $1 || '/%') AND branch = $2
   ```
   where `$1` is `payload.Repository.FullName` (always `owner/repo`).
3. For each connection, apply path filtering (Section 7.1), then independently: create a build record, cancel superseded builds, and enqueue a River job.

Each blueprint's build is fully independent with its own build record and job.

### 7.1 Path Filtering

Before enqueuing a build for a connection, the handler MUST check whether the push touched any files under that connection's `repoSubPath`.

The push payload includes the union of `added`, `removed`, and `modified` file paths across all commits in the push. A connection is eligible for a build if at least one changed file path starts with `{repoSubPath}/`.

Connections where `repoSubPath` is empty (repository root) are always eligible — every push to the branch triggers a build.

If no changed files fall under `repoSubPath`, the connection is skipped and no build record is created.

Path filtering only applies to webhook-triggered builds. Manual rebuilds bypass path filtering entirely.

---

## 8. Disconnect

On disconnect, the handler MUST delete the connection row, then query whether any other connection to the same `repoBase` remains across all accounts. If none remain, call the GitHub API to delete the webhook.

`CountByRepoBase` MUST be added to the store:
```sql
SELECT COUNT(*) FROM github_connections
WHERE repo_full_name = $1 OR repo_full_name LIKE $1 || '/%'
```

---

## 9. AGENT.md Cache Key

`GitHubStatus` caches the draft `AGENT.md` content in Redis. The cache key uses `repo_full_name` directly — it already encodes the subpath:

```
astro:github:agent-md:{conn.RepoFullName}:{branch}
```

Existing root connections (`owner/repo`) produce keys identical in structure to before.

---

## 10. Status Response

`GET /api/v1/agents/:account/:name/github` returns `repo_full_name` as stored (may include subpath segments). No new fields.

---

## 11. Account Connections

`GET /api/v1/accounts/:account/github/connections` returns `repo_full_name` as stored for each connection. No new fields.

---

## 12. Scan

`GET /api/v1/accounts/:account/github/scan` accepts `repo_full_name` as a query parameter. When it contains subpath segments, it scans `{repoSubPath}/astropods.yml` instead of `astropods.yml`.

---

## 13. Files

| File | Change |
|------|--------|
| `apps/astro-server/internal/githubconnection/store.go` | Add `GetByRepoBase(repoBase string)` method; add `CountByRepoBase(repoBase string)` method; update `GetByRepo` → `ListByRepoAndBranch(repoFullName, branch string)` using prefix query |
| `apps/astro-server/internal/githubconnection/store_test.go` | Add `TestStore_GetByRepoBase`, `TestStore_CountByRepoBase`, `TestStore_ListByRepoAndBranch` |
| `apps/astro-server/handlers/github.go` | Webhook dedup via `GetByRepoBase`; fan-out with prefix query and path filtering; conditional webhook delete via `CountByRepoBase` on disconnect; scan subpath from `repo_full_name` param |
| `apps/astro-server/handlers/github_test.go` | Tests for all handler behavior changes |
| `apps/astro-server/internal/githubbuild/fetch.go` | `FetchAstroSpec` and `FetchFileContent` split `repoFullName` internally; derive repo for GitHub API URL and subpath for file path prefixing |
| `apps/astro-server/internal/githubbuild/fetch_test.go` | `TestFetchAstroSpec_SubPath` |
| `apps/astro-server/internal/githubbuild/builder.go` | `RunJob` splits `repoFullName` internally; derives repo for git clone URL and subpath for `build.Context` and `build.Dockerfile` prefixing |
| `apps/astro-server/internal/githubbuild/builder_test.go` | Tests for context and dockerfile path prefixing |

---

## 14. Migration

No schema changes are required. Existing `repo_full_name` values (`owner/repo`) are valid under the new format and continue to work as root connections. No data migration is needed.

