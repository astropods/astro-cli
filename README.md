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

Run the full platform locally with a single command:

```bash
./scripts/local-dev.sh
```

This starts Traefik (port 80), the server (port 8080), and the client (port 5173), then builds the `ast-dev` CLI pointed at `http://localhost`. Everything is available at http://localhost — `/api` routes to the server, everything else to the client.

| Endpoint | Service |
|---|---|
| http://localhost | astro-client |
| http://localhost/api | astro-server |
| http://localhost:8090/dashboard | Traefik dashboard |
| `apps/astro-cli/bin/ast-dev` | local CLI |

Press `Ctrl+C` to stop all services.

### Running client or server independently

If you only need one service:

**Client**

```bash
moon run astro-client:dev
```

Opens http://localhost:5173. Defaults to `VITE_API_URL=http://localhost:8080`; override in `.env` if needed.

**Server**

```bash
moon run astro-server:dev
```

Starts Postgres, runs migrations, and the server with hot reload on http://localhost:8080.

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

## Smoke Tests

End-to-end tests that run against `astropods.com` (prod) and `astropod.ai` (preview). They cover unauthenticated public pages, the authenticated app, CLI install and push, the full deploy flow, and chat.

### Setup

Add credentials to `.env.local` at the repo root:

```
ASTRO_TEST_EMAIL=your@email.com
ASTRO_TEST_PASSWORD=yourpassword
ASTRO_TEST_HOST=https://astropods.com   # or https://astropod.ai for preview
ASTRO_ENV=prod                          # or preview
```

### Running

```bash
bun install
bunx playwright test --config=playwright.prod.config.ts
```

Run a specific project:

```bash
# Blueprint pages — no login needed, runs on all envs
bunx playwright test --config=playwright.prod.config.ts --project=blueprints

# Marketing site — prod only
bunx playwright test --config=playwright.prod.config.ts --project=marketing-site

# CLI + deploy flow — requires credentials
bunx playwright test --config=playwright.prod.config.ts --project=cli

# Skip upstream dependencies
bunx playwright test --config=playwright.prod.config.ts --project=app.chat --no-deps

# Watch with UI
bunx playwright test --config=playwright.prod.config.ts --ui
```

### Test suites

| Project | Tests | Auth | Envs |
|---|---|---|---|
| `marketing-site` | Public marketing pages | None | prod only |
| `blueprints` | Blueprint detail page, deploy redirect | None | all |
| `auth` | Authenticated app loads at root | Session from setup | all |
| `cli` | Install `ast`/`ast-preview`, device-flow login, `bp list`, `push` | Session from setup | all |
| `app.deploy` | Deploy flow, captures deployment slug | Session from setup | all |
| `cli.post-deploy` | `ast agent list` confirms deployment slug | None | all |
| `app.post-deploy` | Polls `/agents` until Hello Astro is Active (14 min) | Session from setup | all |
| `app.chat` | Opens chat popup, sends message, asserts echo | Session from setup | all |
| `app.secrets` | Variables & secrets, verifies auto-fill on deploy page | Session from setup | all |

If login fails, all projects that depend on `setup` are skipped automatically.

Tests run automatically every 15 minutes via the **"Smoke tests"** GitHub Actions workflow. They also run against preview after every deploy to `main`.

## Deployments

| Environment       | URL                   | Notes                                                  |
| ----------------- | --------------------- | ------------------------------------------------------ |
| Preview / Staging | https://astropod.ai   | Requires beta VPN                                      |
| Production        | https://astropods.com | Invite-only — contact a Postman team member for access |

**Preview** — Automatically deployed on every merge to `main`. No manual action required.

**Production** — Deployed manually. After changes land in `main` and are verified in preview, go to the **Actions** tab in GitHub, select the **"Deploy (Prod)"** workflow, and click **"Run workflow"**. You must select at least one service to deploy: `astro-server`, `astro-client`, or `astro-registry`.

To release the CLI to production, use the separate **"Release CLI (Prod)"** workflow.