# Fix astro-client Docker build failure

## Summary

The astro-client Docker build broke because `public/assets` is a symlink to the repo-root `assets/` directory, which isn't copied into the build context. Vite's `prepare-out-dir` step calls `statSync` on every entry in `public/` and fails with ENOENT on the dangling symlink.

## Design

Added `RUN rm -f apps/astro-client/public/assets` in the Dockerfile before the Vite build step. Production serves these assets from the CDN, so they are not needed in the container image.

## Migration

No changes required.
