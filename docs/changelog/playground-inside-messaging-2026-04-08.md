## Summary

Playground was a separate Docker image and submodule that proxied API requests to the messaging service via nginx. This split deployment added operational overhead and required coordinating two images whenever either service changed. The playground is now bundled directly into the messaging binary and served from its root when web mode is enabled.

## Design

The playground source lives in its own repo (`astropods/playground`) and is referenced as a git submodule inside the messaging repo. During the Docker build, a Bun stage compiles the playground's `dist/`, which is then copied into the Go build context before `go build`. Go's `//go:embed` bakes the assets into the binary at compile time — no runtime file serving or external volume needed.

The web adapter gains two new routes, registered after all `/api/*` routes:

- `GET /env-config.js` — served inline with `API_URL: ""` since the UI and API share the same origin
- `GET /` (catch-all) — serves static assets directly; any path without a matching file falls back to `index.html` for client-side routing

A new env var `WEB_SERVE_PLAYGROUND=true` gates the UI routes, so the image can still be used as a pure API server with no change in behavior by default.

The monorepo removes `modules/playground` as a standalone submodule and its dedicated `deployment:playground` Moon task. The `deployment:messaging` task now runs `git -C modules/messaging submodule update --init` before the Docker build to ensure the nested playground submodule is populated.

## Migration

No action required for existing deployments — `WEB_SERVE_PLAYGROUND` defaults to `false`. To enable the bundled UI, set both `WEB_ENABLED=true` and `WEB_SERVE_PLAYGROUND=true` on the messaging container and remove the separate playground container.
