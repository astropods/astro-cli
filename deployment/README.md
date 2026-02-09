# Deployment

Dockerfiles and moon tasks for building Astro service images. Used by CI and to support `ast dev --local`.

## Build images with moon

From the **repository root** (same as [building the CLI](apps/astro-cli/README.md)):

```bash
# Messaging service (gRPC + HTTP sidecar)
moon run deployment:messaging

# Playground (web UI for agents)
moon run deployment:playground

# Remove astro Docker images (messaging, server, registry, playground; local and ghcr.io/saswatds/*)
moon run deployment:clean
```

Images are tagged `astro-messaging:latest` and `astro-playground:latest`. Use them with:

```bash
ast dev --local
```

(`--local` strips the remote registry prefix from compose so those local tags are used and no pull is done.)

## Dockerfiles

| File | Image | Source |
|------|--------|--------|
| `Dockerfile.astro-messaging` | astro-messaging | packages/astro-messaging |
| `Dockerfile.astro-playground` | astro-playground | packages/astro-playground-web |
| `Dockerfile.astro-registry` | astro-registry | apps/astro-registry |
| `Dockerfile.astro-server` | astro-server | apps/astro-server |

Build context for all is the workspace root (so `COPY packages/...` works).

## Packages

Monorepo packages for the Astro platform: agent SDK, graph/workflows, messaging client, engine, types, and related libraries.

### How local refs work

Package deps on `@saswatds/*` use **`workspace:*`**, so they must resolve to the local `packages/*` in this repo, not the registry. That only happens when the install is run from the **repository root** (where the root `package.json` has `"workspaces": ["packages/*", "apps/*"]`). Then Bun links workspace packages and every `@saswatds/*` import points at local source.

- **Leaf:** `astro-types` has no `@saswatds` deps; it can build on its own once deps (e.g. zod) are installed.
- **Others:** Packages like `astro-nodes`, `astro-agent` depend on `@saswatds/astro-types` etc. They build from **local refs** only if you ran `bun install` from the repo root first. If you run install from a single package dir (e.g. `packages/astro-nodes`), those deps resolve from the registry and you can get version/behavior mismatches.

So: **always run `bun install` from the repository root**, then build.

### Building

From the **repository root** (same as CI):

```bashgit
bun install
bun run build
```

`bun run build` runs `moon run :build --query "language=typescript AND projectType=library"` (packages only, no apps). Moon runs builds in dependency order (e.g. `astro-types` before `astro-nodes`).

Single package (and its deps):

```bash
bun install
moon run astro-agent:build
```

### Local development (`ast dev --local`)

With `ast dev --local`, the CLI runs the agent as a local process and loads `@saswatds/astro-agent`, `astro-graph`, and `astro-messaging` from **`ASTRO_SOURCE`**. Those packages must have **`dist/`** built first.

From the repo root:

```bash
export ASTRO_SOURCE=/path/to/astro

bun install
bun run build
# or only the three the agent needs:
# moon run astro-agent:build astro-graph:build astro-messaging:build

ast dev --local
```
