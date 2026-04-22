# GitHub Monorepo — Frontend Subdirectory Support

## Summary

Extends the GitHub connection UI to support monorepo subpath connections. Users can now specify a subdirectory when linking a blueprint to a GitHub repo, so only changes within that path trigger a build. This completes the frontend wiring for the backend monorepo support landed in `feat/github-mono-repo`.

## Design

### Directory picker

A new `SubpathPicker` component provides a searchable dropdown for selecting a repository subdirectory. It calls a new `GET /accounts/:account/github/dirs?repo=&ref=` endpoint that uses the GitHub recursive Git Trees API to fetch all directories in one round-trip, then filters client-side as the user types. Free-text entry is also supported for paths not in the dropdown.

`SubpathPicker` is shared between two connection surfaces:

- **Blueprint detail sidebar** — appears below the repo/branch selectors in the connect dialog, slides in after a repo is chosen
- **New blueprint wizard** — same behavior in the source step; the selected subdirectory is shown in the `LinkConfirmDialog` confirmation before publishing

### Subpath encoding

The selected subdirectory is appended to `repo_full_name` before the link call (`owner/repo/svc/agent`), matching the backend's existing encoding. No new API fields were needed.

### GitHub link URL fix

The connected repo link in the blueprint sidebar previously constructed `https://github.com/{repo_full_name}`, which is invalid for subpath connections. It now links to `https://github.com/{base}/tree/{branch}/{subPath}` when a subpath is set, pointing directly to the correct directory in GitHub's tree view.

### New backend endpoint

`GET /accounts/:account/github/dirs` — returns directory paths for a repo at a given ref. Added to the GitHub client (`GetDirs`), handler (`GitHubAccountListDirs`), and route registration.

## Migration

No migration required. Existing root connections are unaffected.
