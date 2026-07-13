# CLI serves the chat UI and proxies to the messaging sidecar

## Summary

`ast dev` / `ast project` now serve astro-client's chat UI from the CLI itself
instead of the messaging container's bundled playground. The CLI hosts the SPA on
`http://localhost:3100` (labeled "(chat)" in the ready block) and proxies the
deployment-scoped chat/messaging API to a local messaging sidecar. The sidecar is
reduced to the chat **backend** only and persists conversation history to SQLite on
a named Docker volume, so history survives container restarts and across `ast dev`
sessions. This deprecates the messaging-served playground; the sidecar no longer
serves a UI.

## Design

- **UI embedded in the CLI.** astro-client produces a chat build
  (`astro-client:build-chat-embed`) that is copied into
  `apps/astro-cli/internal/chatui/webdist/` and compiled in via `//go:embed`. A
  detached `chatui-serve` worker serves the SPA on `:3100` and survives background
  mode; foreground / `--local` runs stop it on teardown. The worker's lifecycle is
  hardened against the detach: startup displaces any orphaned worker from a prior
  session and then probes an internal `/__chatui/health` endpoint (matching the
  spawned pid) so a failed bind or a stale worker holding `:3100` surfaces a
  warning instead of silently advertising a dead/stale chat URL; teardown verifies
  the recorded pid is still a live `chatui-serve` process (guarding against pid
  reuse) and signals the whole process group.

- **Synthesized deployment endpoints.** The chat SPA expects to talk to a
  deployment, so the CLI serves a small local shim: `/api/v1/deployments/summary`
  (a single "local" account), `/api/v1/deployments?account=local` (one deployment,
  `messaging_web_configured: true`), and `/api/v1/deployments/local/status`
  (`{"value":"active",...}`). Everything under `/messaging/*` is proxied.

- **Backend-only sidecar.** The `astro-messaging` sidecar runs with
  `WEB_SERVE_PLAYGROUND=false` and is published on host port `3110`; the CLI
  proxy targets it. `3100` deliberately belongs to the CLI-served UI, not the
  sidecar.

- **Persistent chat store.** The sidecar writes SQLite to `CHAT_DB_PATH=/data/chat.db`
  on a per-agent named volume (`<agent>-chat-data`), mounted at `/data`, matching
  astro-server's deployed sidecar. Sidecar chat persistence (messaging #61) and its
  server companion (astro #1435) are already merged, so the published
  `astropods/messaging:latest` has the chat endpoints — no messaging-image build or
  `dev.overrides.messagingImage` is needed.

- **Volume ownership init container.** The published messaging image runs as the
  non-root `astro` user (uid 1000) and does not pre-create `/data`. Docker
  initializes a fresh named-volume mountpoint as `root:root`, so the sidecar cannot
  create `/data/chat.db` (`SQLITE_CANTOPEN`) and crashes on startup, which cascades
  into the agent exhausting its gRPC retries. To make the published image work
  out-of-the-box, the CLI now emits a one-shot `astro-messaging-init` service that
  reuses the messaging image, runs as root, `chown`s the volume to uid 1000, and
  exits; the sidecar `depends_on` it with `service_completed_successfully`. It is
  the docker-compose equivalent of the deployed pod's `securityContext.fsGroup=1000`
  (which the kubelet uses to chown the shared agent PVC in k8s —
  `apps/astro-server/internal/k8s/security.go`), and keeps the sidecar itself
  non-root.

## Known compromise / follow-up

The `astro-messaging-init` chown container is a local-dev-only shim for the
fresh-named-volume `root:root` init that only occurs under docker-compose;
deployment needs no equivalent, since k8s chowns the shared agent PVC via the pod's
`fsGroup=1000` (`apps/astro-server/internal/k8s/security.go`). It can be retired once
the published messaging image pre-creates `/data` owned by uid 1000 — Docker seeds a
fresh named volume's ownership from the image directory, so the sidecar could then
write the SQLite store directly and the init service and its `depends_on` could be
dropped.

Secondary: the CLI now probes the chat-UI worker's own `/__chatui/health` before
trusting `:3100`, but the ready gate still prints "ready (chat)" without confirming
the messaging **sidecar** is alive — it can report success while the sidecar has
crashed. Extending the readiness probe to the sidecar (or gating the ready block on
its health) remains a follow-up.

## Migration

No user action. Developers rebuild the CLI so it embeds the current chat UI:
`moon run astro-cli:link` (chains `astro-client:build-chat-embed` →
`astro-cli:embed-chat-ui` → build). Agents only need the web adapter in dev
(`dev.interfaces.messaging.adapters: [web]`); `ast dev` pulls
`astropods/messaging:latest` from the registry.
