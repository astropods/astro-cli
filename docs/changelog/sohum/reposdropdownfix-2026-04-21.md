# Fix: GitHub repo dropdown empty in blueprint detail panel

## Summary

After the GitHub Search API migration (`923993d7`), the per-agent `/github/repos` endpoint was changed to require a `?q` query param — an empty query now returns `[]` immediately instead of listing repos. The `RepoSelectorDialog` in `GitHubConnectionPanel` called this endpoint with no query, so the dropdown always appeared empty and users could not connect a repository from the blueprint detail sidebar.

## Design

`RepoSelectorDialog` now uses `useGitHubAccountRepos` (the account-level endpoint at `/accounts/:account/github/repos`) instead of `useGitHubRepos`. The account-level endpoint handles an empty query correctly — it returns all repos sorted by push date. The old `<Select>` dropdown is replaced with `<RepoPicker>`, the same search-driven autocomplete component used in the blueprint creation wizard. This gives users the same search-as-you-type experience in both places.

## Migration

No action required.
