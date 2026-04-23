# GitHub Monorepo — Frontend Subdirectory Support

## Summary

Extends the GitHub connection UI to support monorepo subpath connections. Users can now specify a subdirectory when linking a blueprint to a GitHub repo, so only changes within that path trigger a build. This completes the frontend wiring for the backend monorepo support landed in `feat/github-mono-repo`.

## Design

### Subpath entry

Rather than a directory browser backed by an API call, the subdirectory is entered as free text directly in the repo picker. On link, the server validates the path exists in the repo via the GitHub Contents API (`PathExists`) and returns 422 with a descriptive error if it does not. This removes a round-trip during picker interaction and keeps the UI simple.

### Subpath encoding

The subdirectory is appended to `repo_full_name` before the link call (`owner/repo/svc/agent`), matching the backend's existing encoding. No new API fields were needed.

### Unified RepoPicker

The repo picker UI is consolidated into a single `RepoPicker` component shared between two connection surfaces:

- **New blueprint wizard** — repo, branch, and optional subdirectory selectors in the source step
- **Blueprint detail sidebar** — same selectors in the connect dialog

### Server-side validation

`GitHubLink` validates the `repo_full_name` format (rejecting empty segments, `.`, `..`) and, when a subpath is present, calls `PathExists` against the GitHub Contents API before installing the webhook. Invalid or missing paths are rejected before any webhook is created.

### GitHub link URL fix

The connected repo link in the blueprint sidebar now points to `https://github.com/{base}/tree/{branch}/{subPath}` when a subpath is set, rather than the invalid `https://github.com/{repo_full_name}`.

## Migration

No migration required. Existing root connections are unaffected.
