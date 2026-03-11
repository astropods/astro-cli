# Astro

A platform for deploying and running AI agents. Includes the `ast` CLI, agent runtime, platform UI (client + server), and messaging adapters (Slack, MCP, web).

## Prerequisites

- **Bun** — JavaScript runtime
- **Go** 1.24+ — for building the CLI
- **Docker** — for `ast dev` and `ast dev --local`

### Install Bun

```bash
curl -fsSL https://bun.com/install | bash
```

This installs `bun` to `~/.bun/bin/bun` by default. Add it to your PATH:

```bash
export PATH="$HOME/.bun/bin:$PATH"
```

### Install Moon (optional, for build tasks)

```bash
bash <(curl -fsSL https://moonrepo.dev/install/moon.sh)
export PATH="$HOME/.moon/bin:$PATH"
```

Or use via bun: `bun x moon run <task>`.

## Setup

```bash
git submodule update --init --recursive
bun install
git config core.hooksPath .githooks
```

The pre-commit hook runs `gofmt` on staged Go files.

## Project Structure

```
├── apps/
│   ├── astro-cli/          # ast CLI (Go)
│   ├── astro-client/       # React frontend application
│   ├── astro-queen/        # Bubbletea TUI admin client (Go)
│   ├── astro-registry/     # Container registry proxy (Go)
│   └── astro-server/       # Platform backend — agent registry, deployments, auth (Go)
├── packages/
│   ├── astro-collector/    # OpenTelemetry Collector distribution
│   ├── astro-identity-gen/ # Procedural SVG avatar generator
│   ├── astro-proto/        # Protobuf definitions and generated gRPC code
│   ├── astro-spec/         # YAML spec parser and types for astropods.yml
│   └── astro-theme/        # Shared UI theme
├── modules/                # Git submodules
│   ├── adapters/           # Messaging adapters (Slack, MCP, etc.)
│   ├── agents/             # Agent examples
│   ├── cli-public/         # Public ast CLI repo
│   ├── messaging/          # Messaging SDK
│   ├── playground/         # Chat UI for agents (used in ast dev)
│   └── website/            # Astro marketing website
├── deployment/             # Dockerfiles and moon tasks for service images
└── docs/                   # Internal documentation and guides
```

## Astro AI Service Development

### Client

```bash
moon run astro-client:dev
```

Open http://localhost:5173. Defaults to `VITE_API_URL=http://localhost:8080`; override in `.env` if needed.

### Server

```bash
moon run astro-server:dev
```

Starts Postgres, runs migrations, and the server with hot reload on http://localhost:8080. Run the client in a separate terminal to use the full platform UI.


## Astro Agent Local Development

Run an agent as a local process with hot-reload, using local packages and Docker images from this repo. Useful for developing agents and packages together.

**Prerequisites:** Packages and Docker images must be built first.

```bash
# 1. Build packages
bun install
bun run build

# 2. Build SDKs required by agents
moon run messaging:sdk-build
moon run adapters:build

# 3. Build Docker images (messaging sidecar, playground)
moon run deployment:messaging
moon run deployment:playground

# 4. Build the CLI
moon run astro-cli:build
```

**Run from an agent project** (e.g. `example-agent`):

```bash
export ASTRO_ROOT=/path/to/astro   # repo root
./apps/astro-cli/bin/ast-dev dev --local
```

Or add the CLI to PATH and run:

```bash
export ASTRO_ROOT=/path/to/astro
export PATH="$PWD/apps/astro-cli/bin:$PATH"
cd example-agent
ast-dev dev --local
```

## Deployments

| Environment | URL | Notes |
|-------------|-----|-------|
| Preview / Staging | https://astropod.ai | Requires beta VPN |
| Production | https://astropods.ai | Invite-only — contact a Postman team member for access |

**Preview** — Automatically deployed on every merge to `main`. No manual action required.

**Production** — Deployed manually. After changes land in `main` and are verified in preview, go to the **Actions** tab in GitHub, select the **"Deploy (Prod)"** workflow, and click **"Run workflow"**. You must select at least one service to deploy: `astro-server`, `astro-client`, or `astro-registry`.

To release the CLI to production, use the separate **"Release CLI (Prod)"** workflow.
