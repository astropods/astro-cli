# Move the CLI chat-embed step to the monorepo side

## Summary

The astro CLI (`modules/astro-cli`, `astropods/astro-cli`) is being prepared to go
open source. The chat SPA is proprietary (astro-client) and must never live in or
be referenced by the public CLI repo. Previously the CLI's own `moon.yml` owned
the `embed-chat-ui` task, which built astro-client and copied its bundle into the
CLI. That coupled the (soon-public) CLI repo to the private astro-client.

## Design

The embed step now lives on the private side:

- `apps/astro-client/moon.yml` gains an `embed-into-cli` task that builds the chat
  bundle (`build-chat-embed`) and copies it into
  `modules/astro-cli/internal/chatui/webdist`.
- `release-cli-prod.yml` and `release-cli-preview.yml` call
  `moon run astro-client:embed-into-cli` instead of `astro-cli:embed-chat-ui`.

The CLI repo's `moon.yml` no longer has the embed task or a build dependency on it.
A plain CLI build embeds only the `.gitkeep` placeholder and the chat route returns
503; the real SPA is baked in only by these release workflows. This severs the last
astro-client reference from the public CLI while keeping the release output
identical.

The CLI's Go module was also renamed to `github.com/astropods/astro-cli`
(astropods/astro-cli#4), so the release ldflags here now inject into
`github.com/astropods/astro-cli/internal/buildinfo.*` instead of the old
`github.com/astropods/astro/apps/astro-cli/...` path. This must land with the
same submodule bump.

## Migration

Coordinated with astropods/astro-cli#4 (removes `embed-chat-ui` from the CLI).
Land this monorepo change first, then bump the `modules/astro-cli` submodule
pointer to the astro-cli commit that drops the task. A release cut after the
pointer bump but before this change would produce binaries with no chat UI.

Developers who want the embedded chat in a local `ast-dev` now run
`moon run astro-client:embed-into-cli` before building (a plain build 503s on the
chat route, which is fine for CLI work).
