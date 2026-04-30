# Subpath Path Filtering for Webhook-Triggered Blueprint Builds

## Summary

In a monorepo setup, every push to a tracked branch triggered a build for all connected blueprints — including subpath connections (e.g. `owner/repo/svc`) even when the push only touched unrelated files. This caused unnecessary K8s build jobs, wasted compute, and noisy build history.

## Design

Filtering runs inside the build worker, immediately after the GitHub token is fetched and before the K8s job is created. For connections with a subpath, the worker:

1. Queries `github_builds` for the most recently registered commit SHA for the connection (already stored, no schema change).
2. Calls the GitHub Contents API twice — once per commit — to fetch the directory listing of the subpath's parent and read the tree SHA for the subpath entry.
3. Compares the two tree SHAs. Equal SHA means identical contents under the subpath since the last build.

If the SHAs match, the build record is marked `skipped` with the message `"no changes under {subpath}/"` and the River job is cancelled before any K8s work starts. The skipped build is visible in the build history UI.

Two targeted API requests regardless of subpath depth — no file lists, no diffs, no tree recursion. On any API error or missing subpath, the build proceeds (conservative fallback, never silently skips).

Manual rebuilds bypass filtering entirely — the last registered commit lookup only runs for webhook-triggered builds that have a prior registered build to compare against.

## Migration

No schema changes. Existing connections and builds are unaffected. Subpath connections will now show `skipped` builds in history when a push doesn't touch their directory.
