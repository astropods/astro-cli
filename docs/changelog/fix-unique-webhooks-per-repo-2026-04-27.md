# Fix: One Webhook Per GitHub Repo

## Summary

When two Astro accounts both connected to the same GitHub repo, each created its own webhook. GitHub fired both on every push — double API traffic, duplicate entries in the delivery log, and confusing redundancy for repo admins. Webhook ownership was account-scoped, so the existing dedup logic only prevented duplicates within a single account, not across accounts.

## Design

Webhook ownership moves to a new global `github_webhooks` table keyed on `repo_base` (the `owner/repo` portion). The PRIMARY KEY constraint makes it structurally impossible to represent two webhooks for the same repo.

`webhook_id` and `webhook_secret` are dropped from `github_connections`. A connection record now expresses only intent — "this agent wants builds from this repo+branch" — and nothing more.

A new `internal/githubwebhook` package owns the three operations against the new table:

- `Get(repoBase)` — look up the shared secret for HMAC verification
- `Insert(repoBase, webhookID, secret)` — registers a new webhook; uses `ON CONFLICT (repo_base) DO NOTHING` and returns `inserted=false` when another instance already holds the row
- `DeleteIfNoConnections(repoBase)` — atomically deletes the row only when no `github_connections` rows reference that repo, returning the `webhook_id` for the caller to remove from GitHub

**GitHubLink** saves the connection first, then calls `webhookStore.Get(repoBase)`. If a webhook already exists globally, no GitHub API call is made. If not, it creates one and inserts it. When two instances race, both create a GitHub webhook and both attempt `Insert`. The loser gets `inserted=false` (0 rows affected), deletes its orphaned GitHub webhook, and proceeds — no error-code inspection needed.

**GitHubWebhook** (inbound push handler) calls `webhookStore.Get` for HMAC verification and `ghStore.ListByRepoAndBranch` (new global method, no account filter) for fan-out. A single verified push now triggers builds in every account connected to that repo and branch — per-account scoping was only meaningful when secrets lived on `github_connections`.

**GitHubDisconnect**, **GitHubAccountDisconnect**, and **ArchiveAgent** all replace the `CountByRepoBaseForAccount` + conditional delete pattern with `webhookStore.DeleteIfNoConnections`. The atomicity of that operation eliminates the window between count-check and delete.

## Migration

Schema changes are applied via Atlas against `sql/astro-server/schema.sql`. No data backfill is needed — only one internal user has GitHub connections in production; they will be asked to reconnect after deploy, which re-creates the webhook via the new code path.
