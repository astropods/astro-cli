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

Four new store methods replace `GetByRepo`:

- **`GetByRepoBase(repoBase)`** — returns any connection matching the base repo (exact or prefix). Used to retrieve the webhook secret for HMAC verification on push.
- **`GetByRepoBaseForAccount(accountID, repoBase)`** — same as above, scoped to one account. Used for webhook dedup on link.
- **`CountByRepoBaseForAccount(accountID, repoBase)`** — counts connections for a given account and base repo. Used in disconnect to decide whether to remove the webhook.
- **`ListByRepoAndBranch(repoFullName, branch)`** — returns all connections for a base repo+branch via prefix query (`WHERE repo_full_name = $1 OR repo_full_name LIKE replace($1, '_', '\_') || '/%' ESCAPE '\'`). `_` is escaped because GitHub repo names can contain underscores; `%` is not a valid GitHub name character so no escaping is needed for it. Used for webhook fan-out.

### Spec and build path resolution

`FetchFileContent` and `FetchAstroSpec` split `repoFullName` internally: the base repo is used in the GitHub API URL, and the subpath prefixes the file path (`svc/astropods.yml` instead of `astropods.yml`).

`RunJob` (BuildKit K8s jobs) uses the base repo for the git clone URL and a new `effectivePaths` helper to resolve workspace paths: `build.Context = "."` inside subpath `svc` becomes context `/workspace/svc`, and `Dockerfile` becomes `--opt filename=svc/Dockerfile`.

### Linking

`repo_full_name` is validated on link: must have at least two slash-separated segments, each containing only `[A-Za-z0-9._-]`, with a maximum length of 100 characters per segment. This blocks shell and URL metacharacters that could be injected into GitHub API calls or git-clone commands.

Webhook deduplication: before creating a new webhook, the handler calls `GetByRepoBaseForAccount` to check if the same account already has a connection to the same base repo. If found, the new connection inherits the existing `webhook_id` and `webhook_secret` — no new GitHub webhook is created. This matches the pre-existing behavior: one webhook per `(account, base repo)`. Subpath connections within an account share the base repo's webhook; connections from different accounts each have their own independent webhook.

### Webhook fan-out

On push, `GetByRepoBase` retrieves the shared webhook secret for HMAC verification. Then `ListByRepoAndBranch` returns all matching connections. Every connection receives a build — there is no subpath path filtering. Filtering based on the push payload's commits list was removed because GitHub truncates that list at 20 entries, making it unreliable. If any connection fails to enqueue, the handler returns 500 so GitHub's redelivery UI surfaces the partial failure.

### Disconnect (account-level)

The connection row is deleted first, then `CountByRepoBaseForAccount` determines whether to remove the webhook. Deleting before counting ensures the post-deletion count is accurate. The webhook is only removed when the disconnecting account has no remaining connections to that base repo — subpath connections within the same account keep the webhook alive.

## Migration

No schema changes. Existing root connections (`owner/repo`) continue to work without any modification.
