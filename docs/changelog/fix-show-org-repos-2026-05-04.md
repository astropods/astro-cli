# fix-show-org-repos

## Summary

GitHub repo search only supported personal repositories. This adds org repo support by fetching the user's org memberships and including them in the search scope. Because this requires a separate API call before every search, org lookups are cached in Redis to avoid hitting GitHub on every keystroke as users type in the repo picker.

## Design

### GitHub client

A new `GetOrgs(ctx) ([]string, error)` method calls `/user/orgs?per_page=100` and returns org login names. `SearchRepos` now accepts a pre-fetched `orgs []string` parameter; each org is appended as an `org:<login>` qualifier in the search query.

The GitHub Search API has no way to automatically include all org repos — there is no "search everything I have access to" qualifier. To keep search on the GitHub side (rather than fetching all repos locally and filtering), we have to enumerate org memberships explicitly and inject one `org:<login>` qualifier per org. That's the separate `GetOrgs` call.

The scope string for all search queries is now:

```
user:<login> fork:true org:<org1> org:<org2> ...
```

Both the empty-query case (sorted by push date) and the name-filter case (`in:name`) use this scope.

### Org caching

`SearchRepos` takes a pre-fetched `orgs []string` instead of calling `GetOrgs` internally, so the org lookup can be cached independently of the search query. `GitHubAccountListRepos` owns the cache read/write: it checks Redis under `astro:github:orgs:<userID>` (1-hour TTL) before hitting the GitHub API. On a cache miss or unmarshal failure it fetches fresh and repopulates. Subsequent keystrokes hit Redis instead of GitHub.

The org cache is invalidated when a user disconnects their GitHub account so that a reconnect with a different account or updated org memberships picks up fresh data immediately rather than waiting for the TTL to expire.

Both `GitHubAccountListRepos` and `GitHubAccountDisconnect` receive the cache as an injected `k8scache.Cache` parameter.

## Migration

The WorkOS GitHub OAuth app must have the `read:org` scope added so the server can call the GitHub `/user/orgs` endpoint. Without it, org membership lookups return empty and org repos remain hidden.
