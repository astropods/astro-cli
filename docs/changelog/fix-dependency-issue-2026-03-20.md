# Fix: uqr missing in astro-client production container

## Summary

The preview site was returning 503 errors due to a missing `uqr` dependency at runtime. The SSR server crashed on startup because it could not resolve the `uqr` package imported by `astro-trading-card`.

## Design

`astro-trading-card` added `uqr` as a dependency in commit `2631a4ab` for QR code rendering. In the monorepo, `bun install` at the root hoists `uqr` into the root `node_modules`, making it available to all workspace packages including `astro-client`. This means the issue is invisible locally.

In the production container, only `astro-client` is present — the monorepo workspace is not. The container runs a clean `bun install` from `astro-client/package.json` only, so `uqr` was never installed. When the SSR bundle tried to import it at runtime, Bun threw a module-not-found error and the server exited with a 503.

The fix is to declare `uqr` as a direct dependency of `astro-client` so it is included in the container's install.

## Docker smoke test

To prevent regressions of this class, a smoke test was added that runs the production image end-to-end:

- **`deployment/smoke-test-astro-client.sh`** — builds the image, starts a container, and polls `http://localhost:13000/` until any HTTP response arrives. A response proves `server.ts` completed its top-level `await import('./build/server/index.js')` without crashing. Missing deps would exit the process before the port is bound, failing the poll.
- **`test-client-docker` CI job** — runs the smoke test on PRs touching the Dockerfile, any `package.json`, or `bun.lock`. Scoped to dependency-related paths to avoid a full Docker build on every source change.
- **`deployment:smoke-test-astro-client` Moon task** — for local verification via `moon run deployment:smoke-test-astro-client`.

## Migration

No action required.
