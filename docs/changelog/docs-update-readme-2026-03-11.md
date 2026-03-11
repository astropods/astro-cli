# Update README project structure and local dev instructions

## Summary

The README project structure diagram was outdated and the local development instructions referenced a non-existent binary.

## Design

**Project structure** — Rewrote the tree diagram to match the actual repo layout. Submodules (`agents`, `adapters`, `messaging`, `playground`, `cli-public`, `website`) moved from `apps/` and `packages/` to `modules/`. Added missing packages (`astro-collector`, `astro-proto`, `astro-spec`, `astro-theme`), missing app (`astro-queen`), and top-level directories (`deployment/`, `docs/`).

**Local dev binary** — Changed `ast` to `ast-dev` in the run examples since the dev build produces `apps/astro-cli/bin/ast-dev`, not `ast`.

**Typo** — Fixed "SKDs" to "SDKs".

## Migration

No action required.
