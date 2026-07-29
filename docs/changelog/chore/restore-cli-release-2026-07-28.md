# Restore CLI release from the monorepo

## Summary

The astro CLI is released from this monorepo, not from a standalone repo. A prior
change (#1784) had deleted the monorepo's CLI release workflows on the assumption
the CLI would self-release; that direction was reversed. This restores the two
release workflows so the CLI can again be built and published from here.

## Design

`release-cli-prod.yml` (dispatch, `ast`) and `release-cli-preview.yml` (dispatch,
`ast-preview`) both:

1. Fetch the private `modules/astro-cli` submodule.
2. Build astro-client's chat SPA and embed it (`moon run astro-cli:embed-chat-ui`).
3. Build the four OS/arch binaries and upload them to the environment's downloads
   bucket, then invalidate CloudFront.

Two changes from the deleted versions:

- **Dropped the `FleetServerURL` ldflag** and its build-flag check. Fleet was
  removed from the CLI, so `buildinfo` no longer has that symbol and the old
  verify step would fail.
- The top-of-file comments now describe the monorepo-release model instead of the
  abandoned self-release plan.

**Credential.** Fetching the private submodule requires a cross-repo credential:
`CLI_DEPLOY_KEY`, a read-only deploy key on `astropods/astro-cli`. GitHub's
`GITHUB_TOKEN` is scoped to this repo only and cannot read another private repo's
contents, so this is unavoidable while the CLI repo is private. The only keyless
alternative would be making `astropods/astro-cli` public (then the submodule
fetches over HTTPS with no key, like `packages/astro-spec`).

## Migration

Operators: (re)create the `CLI_DEPLOY_KEY` secret (a read-only deploy key on
`astropods/astro-cli`) if it was removed. Both workflows are manual dispatch; add
a `push` trigger to preview if every merge to `main` should cut a preview build.
