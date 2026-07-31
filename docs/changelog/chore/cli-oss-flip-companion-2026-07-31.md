# Fetch the CLI submodule keyless over HTTPS

## Summary

The astro CLI (`modules/astro-cli`, `astropods/astro-cli`) is going open source and
its read-only deploy key was retired. The prod and preview release workflows used
to fetch the private submodule over SSH with that key; with the key gone they need
a keyless path. Once astro-cli is public it can be fetched over anonymous HTTPS
with no credentials, exactly like the already-public `packages/astro-spec`.

## Design

- `.gitmodules` points `modules/astro-cli` at `https://github.com/astropods/astro-cli.git`
  (matching how `packages/astro-spec` is wired), so `git submodule update` needs no key.
- `release-cli-prod.yml` and `release-cli-preview.yml` drop the SSH/deploy-key setup;
  the fetch step is now a plain `git submodule update --init modules/astro-cli`.
- Build, embed, and publish steps are unchanged; release output is identical.

## Migration

This is a flip-companion change and must merge in the same window `astro-cli` is made
public, together with the submodule pointer bump. Keyless HTTPS fetch only works once
the target repo is public; a plain HTTPS `git submodule update` cannot authenticate
against a private repo. Both release workflows are manual (`workflow_dispatch`), so
nothing releases automatically in the interim.

`astro-cli` is published as a single squashed "initial public release" commit, because
its pre-OSS history carried monorepo paths and private component names. Landing this
change therefore pairs with re-bumping the `modules/astro-cli` pointer to that commit.
