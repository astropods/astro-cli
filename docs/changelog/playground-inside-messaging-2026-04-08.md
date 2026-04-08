## Summary

Playground was a separate submodule, Docker image, and CLI command that ran alongside the messaging service. This split required coordinating two containers and exposed the playground on a different port (3000) from the API (3100). The playground is now bundled into the messaging binary and served from the same origin — this removes the standalone submodule, the `ast playground` command, and the separate container from both local dev and production deployments.

## Design

**`modules/playground` removed as standalone submodule.** The playground source still lives in `astropods/playground.git` but is now a nested submodule inside `astropods/messaging.git`. The monorepo no longer tracks it directly.

**`deployment:playground` Moon task removed.** The `deployment:messaging` task now runs `git -C modules/messaging submodule update --init` before the Docker build to ensure the playground submodule is populated in the build context.

**`ast playground` command removed from astro-cli.** The command pulled `astropods/playground:latest` and ran it as a container pointing at a messaging URL. Since the UI is now served from messaging itself, navigating directly to the messaging HTTP endpoint is sufficient.

**Compose builder updated (astro-cli).** The standalone `playground` service is no longer added to the Docker Compose project. Instead, `WEB_SERVE_PLAYGROUND=true` is set on the `astro-messaging` service when the web adapter is enabled. The playground URL in the ready block and auto-open changes from `localhost:3000` to `localhost:3100`.

**K8s deployment updated (astro-server).** `WEB_SERVE_PLAYGROUND=true` is now injected alongside `WEB_ENABLED=true` on the messaging init container when the web adapter is configured.

## Migration

- **`ast dev`** — no change needed in `astropods.yml`. The compose project will automatically serve the playground from the messaging container at `http://localhost:3100`.
- **Production** — remove the standalone playground container. If the messaging container has `WEB_ENABLED=true`, add `WEB_SERVE_PLAYGROUND=true` to serve the UI. Existing deployments without this flag are unaffected.
- **`ast playground` command** — remove any scripts or docs that call it; open the messaging HTTP endpoint directly instead.
