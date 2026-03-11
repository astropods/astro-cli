# Astro CLI — Internal

Internal notes for building, testing, and working on the CLI. For user-facing usage see [README.md](../README.md).

## Prerequisites

- **Go** 1.24+
- **Docker** and Docker Compose
- **moon** — monorepo build tool

### Installing moon

```bash
bash <(curl -fsSL https://moonrepo.dev/install/moon.sh)
export PATH="$HOME/.moon/bin:$PATH"
```

Or via npm/bun: `bun add -d @moonrepo/cli` (then `bun x moon run astro-cli:build`).

## Building

From repo root:

```bash
moon run astro-cli:build
```

Output: `apps/astro-cli/bin/ast-dev`.

To build and symlink into `~/go/bin/ast-dev` (must be on `$PATH`):

```bash
moon run astro-cli:link
```

**Config namespacing**: `ast-dev` stores credentials in `~/.ast-dev/`, separate from production `ast` (`~/.ast/`). After linking, run `ast-dev configure` in your agent project to set up API keys. To share credentials with `ast`, symlink the config: `ln -sf ~/.ast/project-configs.json ~/.ast-dev/project-configs.json`.

## Testing

```bash
cd apps/astro-cli
go test ./...
```

Verbose:

```bash
go test -v ./cmd/...
```

Tests include push version/auto logic, `baseVersion`, `updateSpecVersion`, and `defaultPushTag` (with temporary git repos for git-clean/git-dirty cases).

## Dependencies

- **Cobra** — CLI framework
- **Docker SDK** — Container builds and orchestration
- **Compose Go** — Docker Compose project generation
- **ORAS** (crane) — OCI artifact push (via auth/crane)
- **fsnotify** — File watching for hot reload
- **astro-spec** (internal package) — YAML spec parsing and types

## Project structure

```
apps/astro-cli/
├── main.go                 # Entry point
├── cmd/
│   ├── root.go             # Root command, global flags
│   ├── dev.go              # ast dev — local development, compose, hot reload
│   ├── build.go             # ast build — container builds (BuildKit)
│   ├── push.go              # ast push — version logic, registry push, register
│   ├── push_streaming.go    # Push progress, multi-platform
│   ├── login.go / logout.go # Auth
│   ├── playground.go       # ast playground
│   ├── create.go            # ast create — scaffold new agent
│   └── version.go           # version/commit (ldflags-injected)
├── internal/
│   ├── auth/               # Server/registry auth, token storage, crane
│   ├── compose/             # Compose project generation from spec
│   ├── scaffold/            # Templates for ast create
│   ├── utils/               # Helpers (env, image names)
│   └── watcher/             # File watcher for hot reload
├── go.mod / go.sum
├── moon.yml                 # moon build task
└── docs/
    └── INTERNAL.md          # This file
```

Spec types and parsing live in `packages/astro-spec` (shared with server).

## Architecture

1. **Spec** — `astropods.yml` is parsed by `packages/astro-spec` into structured types.
2. **Dev** — `compose` builder turns the spec into a Docker Compose project; `dev` runs it and optionally runs the agent process locally with a watcher.
3. **Build** — For each component with `container.build`, the CLI invokes Docker/BuildKit with the right context, Dockerfile, secrets (e.g. npm token from env or injected), and platform.
4. **Push** — Each push generates a random 8-character build ID used as the image tag. Images are tagged and pushed (single or multi-platform); spec is pushed as OCI artifact and optionally sent to Astro server for registration.

### Local push (`--local`)

`ast push --local` builds images and registers the spec with a locally running astro-server (`http://localhost:8080`) instead of the remote platform. The remote registry push is skipped.

| Aspect | Normal | `--local` |
|--------|--------|-----------|
| Auth / namespace fetch | Remote registry | Skipped (uses namespace `local`) |
| Build | Yes | Yes |
| Image push | Remote registry | Skipped |
| Image retag | — | Local `docker tag` to registry path |
| Registration server | Remote (from profile) | `http://localhost:8080` |

Because the spec's image references use the full registry path (e.g. `registry.example.com/ns/agent:tag`), the CLI retags each locally-built platform image to that path so the local astro-server (which deploys with `imagePullPolicy: Never`) can resolve them from the local Docker daemon.

Usage:

```bash
# Start local astro-server
cd apps/astro-server && moon run astro-server:dev

# Push to local server
ast push --local
```

Design principles:

- **Declarative** — Infrastructure as code in YAML.
- **Container-native** — Builds and runs everything in Docker.
- **OCI-compatible** — Works with any registry; ORAS for spec artifacts.
