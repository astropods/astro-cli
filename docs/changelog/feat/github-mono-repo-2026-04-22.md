# GitHub Monorepo Support

## Summary

Teams using monorepos were blocked from connecting multiple blueprints to the same GitHub repository. The existing model enforced one connection per repo per account. This change extends `repo_full_name` to encode an optional subpath (`owner/repo/sub/path`), enabling multiple blueprints to share a single repo at different subdirectories.

## Design

### Data model

`repo_full_name` in `github_connections` now stores either `owner/repo` (root, unchanged) or `owner/repo/sub/path` (subdir). Two derived values are used throughout:

- **`RepoBase`** — the first two segments (`owner/repo`). Used for GitHub API calls, git clone, and webhook operations.
- **`RepoSubPath`** — everything after the third `/`, or empty for root connections. Used for file path prefixing.

No schema changes are required. The existing `UNIQUE (account_id, agent_name)` constraint continues to work as-is. The existing `UNIQUE (account_id, repo_full_name)` constraint enforces uniqueness at the `(repo, subpath)` level.

### Store (`githubconnection`)

Three new store methods replace `GetByRepo`:

- **`GetByRepoBase(repoBase)`** — returns any connection matching the base repo (exact or prefix). Used to retrieve the shared webhook secret for HMAC verification.
- **`CountByRepoBase(repoBase)`** — counts all connections for a base repo across all accounts. Used in disconnect to decide whether to remove the webhook.
- **`ListByRepoAndBranch(repoFullName, branch)`** — returns all connections for a base repo+branch via prefix query (`WHERE repo_full_name = $1 OR repo_full_name LIKE $1 || '/%'`). Used for webhook fan-out.

### Spec and build path resolution

`FetchFileContent` and `FetchAstroSpec` split `repoFullName` internally: the base repo is used in the GitHub API URL, and the subpath prefixes the file path (`svc/astropods.yml` instead of `astropods.yml`).

`RunJob` (BuildKit K8s jobs) uses the base repo for the git clone URL and a new `effectivePaths` helper to resolve workspace paths: `build.Context = "."` inside subpath `svc` becomes context `/workspace/svc`, and `Dockerfile` becomes `--opt filename=svc/Dockerfile`.

### Linking

`repo_full_name` is validated on link: must have at least two segments, no `..`, `.`, or empty path components. Validation runs before any external API calls.

Webhook deduplication: before creating a new webhook, the handler calls `GetByRepoBase` to check if any existing connection already has a webhook on the same base repo. If found, the new connection inherits the existing `webhook_id` and `webhook_secret` — no new GitHub webhook is created.

### Webhook fan-out

On push, `GetByRepoBase` retrieves the shared webhook secret for HMAC verification. Then `ListByRepoAndBranch` returns all matching connections. For each connection with a non-empty subpath, a path filter checks whether any changed file (union of `added`, `removed`, `modified` across all commits) starts with `{subPath}/`. Connections that don't match are skipped — no build record is created.

### Disconnect

Delete runs first, then `CountByRepoBase` determines whether to remove the webhook. The webhook is deleted only if count reaches zero.

## Migration

No schema changes. Existing root connections (`owner/repo`) continue to work without any modification.
