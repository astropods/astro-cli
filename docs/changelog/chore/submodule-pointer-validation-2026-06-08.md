# Submodule pointer validation

## Summary

`modules/website` was accidentally pointed at a `modules/blog` commit in #1295 (same regression class as #1264, previously fixed in #1287). Bulk submodule bumps can silently cross-wire paths when the wrong gitlink is staged. This repairs the website pointer and adds CI validation so invalid SHAs fail before merge.

## Design

**Pointer repair** — Revert `modules/website` to `58585d11` (current `astro-ai-website` `main`), the commit recorded correctly in #1287.

**`scripts/validate-submodules.sh`** — Offline check over superproject metadata only: every `.gitmodules` path must have a gitlink at the ref, and no two paths may share the same SHA (catches blog/website mix-ups during bulk bumps). Does not clone or fetch private submodule remotes — CI cannot reach them without deploy keys.

**CI** — `.github/workflows/validate-submodules.yml` runs on PRs and pushes to `main` when `.gitmodules`, any `modules/**` gitlink, or the script changes. Superproject checkout only; no submodule auth required. Developers still use `update-submodules.sh` locally to confirm SHAs exist on remotes.

## Migration

Nothing required. After merge, `bash scripts/update-submodules.sh` should complete without the stale-pointer warning for `modules/website`.
