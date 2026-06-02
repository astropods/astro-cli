# Blueprint sidebar: split repo and directory into separate links

## Summary

The blueprint detail sidebar's "Repository" row showed the repo name with an optional `/<directory>` underneath, but the whole block was a single link to the repo root — so users couldn't jump straight into the subdirectory where the agent actually lives. Now there are two links: the top row goes to the repo URL, and (when the blueprint specifies a directory) a second row links into that subdirectory on the host provider.

## Design

`buildRepoDirectoryUrl(url, directory)` in `SidebarRepository.tsx` maps `(repo url, directory)` to a provider-specific tree URL using `HEAD` as the ref so we don't need to know the default branch name:

- `github.com` / `gist.github.com` → `<repo>/tree/HEAD/<dir>`
- `gitlab.com` → `<repo>/-/tree/HEAD/<dir>`
- `bitbucket.org` → `<repo>/src/HEAD/<dir>`
- unknown providers or unparseable URLs → fall back to the repo URL

The component renders two sibling anchors inside a `<div>` (not a nested `<a>`, which would be invalid HTML). The directory row is indented `pl-[22px]` so the `└─` connector sits under the icon column of the row above, signalling the parent/child relationship visually. Hover underline lives on the directory path span (via `group-hover:underline`) rather than the anchor, because `truncate`'s `overflow: hidden` clips the underline at the default offset — the path span gets `pb-0.5` and `underline-offset-2` so the underline fits inside the box.

Unit tests in `SidebarRepository.test.tsx` cover each provider, `.git` and trailing-slash normalization, leading/trailing slashes on the directory, the `www.` host prefix, unknown providers, and unparseable URLs.

## Migration

No action required.
