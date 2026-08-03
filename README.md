<div align="center">

# astro-cli

**`ast` — the command-line tool for building, running, pushing, and deploying agents on the Astro AI platform.**

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white)](go.mod)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

[Install](https://docs.astropods.com/install-cli) · [CLI Reference](https://docs.astropods.com/cli-reference) · [Build from source](#build-from-source) · [Architecture](#architecture) · [Design](#design-principles)

</div>

---

> 📖 To install a released build, see [**Install the CLI**](https://docs.astropods.com/install-cli).
> For every command and flag, see the [**CLI reference**](https://docs.astropods.com/cli-reference).

## Prerequisites

- **Go** 1.25+
- **Docker** and Docker Compose (for `ast dev` and container builds)

## Build from source

```sh
git clone https://github.com/astropods/astro-cli.git
cd astro-cli
go build -o bin/ast .
```

A plain source build is configured for **local development**: the binary reports
itself as `ast-dev` and defaults to a local server at `http://localhost:8080` (the
server URL and binary name are injected via `-ldflags` at release time). To use the
CLI against the hosted Astro AI platform, install an official release binary - see
[Install the CLI](https://docs.astropods.com/install-cli).

The chat UI is not part of this repository, so a source build serves a 503 on the
chat route. Official release binaries ship with the chat UI embedded.

## Test

```sh
go build ./...
go vet ./...
go test ./cmd/... ./internal/...
```

End-to-end tests live under `e2e/` behind the `integration` build tag:

```sh
go test -tags integration ./e2e/...
```

## Dependencies

- **Cobra** — CLI framework
- **Docker SDK** / **Compose Go** — container builds and Docker Compose generation
- **ORAS** (crane) — OCI artifact push
- **fsnotify** — file watching for hot reload
- **[astro-spec](https://github.com/astropods/astro-spec)** — the `astropods.yml` spec parser and types (a public Go module)

## Project structure

```
.
├── main.go             # entry point
├── cmd/                # commands: dev, build, push, create, login, ...
├── internal/
│   ├── auth/           # server/registry auth, token storage
│   ├── compose/        # Docker Compose project generation from the spec
│   ├── scaffold/       # templates for `ast create`
│   ├── chatui/         # local chat UI server (assets embedded at release time)
│   └── watcher/        # file watcher for hot reload
├── e2e/                # integration tests
└── go.mod / go.sum
```

Spec types and parsing come from the
[astro-spec](https://github.com/astropods/astro-spec) module.

## Architecture

1. **Spec** — `astropods.yml` is parsed by `astro-spec` into structured types.
2. **Dev** — the `compose` builder turns the spec into a Docker Compose project;
   `ast dev` runs it locally.
3. **Build** — for each component with `container.build`, the CLI invokes
   Docker/BuildKit with the right context, Dockerfile, secrets, and platform.
4. **Push** — each push generates a random 8-character build ID used as the image
   tag. Images are tagged and pushed (single or multi-platform); the spec is
   pushed as an OCI artifact and optionally registered with an Astro server.

### Local push (local astro-server)

When the CLI targets a local astro-server (`http://localhost:8080`, `127.0.0.1`, or
`::1`), `push` automatically registers the spec with that server and skips the
remote registry, retagging images locally instead. There is no separate flag - the
behavior is inferred from the target server URL.

| Aspect | Remote server | Local server |
|--------|--------------|--------------|
| Auth / namespace fetch | Remote registry | Skipped (namespace `local`) |
| Build | Yes | Yes (native platform) |
| Image push | Remote registry | Skipped |
| Image retag | n/a | Local `docker tag` to the registry path |
| Registration server | Remote (from profile) | `http://localhost:8080` |

Because the spec's image references use the full registry path (e.g.
`registry.example.com/ns/agent:tag`), the CLI retags each locally built platform
image to that path so a local astro-server can resolve them from the local Docker
daemon.

## Design principles

- **Declarative** — infrastructure as code in YAML.
- **Container-native** — builds and runs everything in Docker.
- **OCI-compatible** — works with any registry; ORAS for spec artifacts.
