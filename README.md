# Astro

A platform for deploying and running AI agents. Includes the `ast` CLI, agent runtime, platform UI (client + server), and messaging adapters (Slack, MCP, web).

## Prerequisites

- **Bun** — JavaScript runtime
- **Go** 1.25+ - for building the CLI and Go services
- **Docker** - for `local-dev.sh` and `ast dev`

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
│   ├── astro-client/       # React web frontend
│   ├── astro-otel/         # OTLP ingest for local AI coding tools (traces/metrics)
│   ├── astro-proxy/        # Local proxy helper
│   ├── astro-queen/        # Bubbletea TUI admin console (Go)
│   ├── astro-registry/     # Container registry proxy with auth (Go)
│   └── astro-server/       # Platform backend: agent registry, deployments, auth (Go)
├── packages/
│   ├── astro-brand-icons/  # Brand icon components
│   ├── astro-collector/    # OpenTelemetry Collector distribution
│   ├── astro-identity-gen/ # Identity asset generation
│   ├── astro-proto/        # Protobuf definitions and generated gRPC code
│   ├── astro-spec/         # YAML spec parser/types for astropods.yml (public submodule)
│   ├── astro-theme/        # Shared UI theme
│   ├── astro-trading-card/ # Agent trading-card UI
│   └── blueprint-jellybean/# Shared UI package
├── modules/                # Git submodules
│   ├── adapters/           # Messaging adapters (Slack, MCP, etc.)
│   ├── agents/             # Agent examples
│   ├── astro-cli/          # ast CLI (Go): build, push, deploy, local dev
│   ├── astro-infra/        # Infrastructure as code
│   ├── blog/               # Astro blog
│   ├── messaging/          # Messaging SDK and sidecar service
│   └── website/            # Astro marketing website
├── deployment/             # Dockerfiles and moon tasks for service images
└── docs/                   # Internal documentation and guides
```

## Astro AI Service Development

There are two ways to run the platform locally. Both need `apps/astro-server/.env` (copy `apps/astro-server/.env.example` and set `DATABASE_URL` to your dev database - it's remote, nothing local starts Postgres; add stage WorkOS credentials for login, see the [astro-server README](apps/astro-server/README.md)) and a running Docker.

### Option A: one command, behind Traefik

```bash
./scripts/local-dev.sh
```

Starts Traefik, astro-server, and astro-client, and builds the `ast-dev` CLI, then fronts everything on a single origin at **http://localhost** (`/api` routes to the server, everything else to the client). This mirrors production most closely. `Ctrl+C` stops all services and tears down Traefik.

| Endpoint | Service |
|---|---|
| http://localhost | astro-client |
| http://localhost/api | astro-server |
| http://localhost:8090/dashboard/ | Traefik dashboard |
| `modules/astro-cli/bin/ast-dev` | local CLI |

### Option B: each service separately, no Traefik

Run each service in its own terminal - simpler, with no Traefik front, but the client and server sit on different ports:

```bash
moon run astro-server:dev   # applies schema + River migrations, hot-reloads on http://localhost:8080
moon run astro-client:dev   # http://localhost:5173
```

The client talks to the backend at `VITE_API_URL` (default `http://localhost:8080`); its Vite dev server proxies `/api`, `/auth`, `/download`, `/install`, and `/webhooks` there, so the browser stays same-origin at `:5173`. Override `VITE_API_URL` in `apps/astro-client/.env`. Build the CLI with `moon run astro-cli:link` (it targets `:8080`).

## Astro Agent Local Development

Develop and run an agent against the local platform with the `ast-dev` CLI. Agents run in Docker, and the CLI pulls `astropods/messaging:latest` automatically, so the common case needs no image builds.

Build and link the CLI once:

```bash
moon run astro-cli:link           # builds ast-dev, symlinks it into ~/go/bin
export PATH="$HOME/go/bin:$PATH"   # if ~/go/bin isn't already on PATH
```

Then, from an agent project (e.g. `example-agent`):

```bash
cd example-agent
ast-dev dev            # start all containers
ast-dev dev logs       # tail agent logs (--all for every service)
ast-dev dev stop       # tear down
```

`ast-dev` targets the local server at `http://localhost:8080` and auto-detects local mode from that URL (native-arch build, retag locally, skip registry push). If the agent enables the `web` messaging adapter, `ast-dev dev` serves the chat UI at http://localhost:3100.

To iterate on the messaging image itself, build it and point the spec at your build:

```bash
moon run deployment:messaging     # builds and tags astropods/messaging:latest
# then in astropods.yml:
#   dev:
#     overrides:
#       messagingImage: "messaging:latest"
```

See [docs/04-guides/local-development.md](docs/04-guides/local-development.md) for the full runbook.

## Smoke Tests

End-to-end tests that run against `astropods.com` (prod) and `astropod.ai` (preview). They cover unauthenticated public pages, the authenticated app, CLI install and push, the full deploy flow, and chat.

### Setup

Smoke tests use a WorkOS test account whose credentials are passed as environment variables (`ASTRO_TEST_EMAIL`, `ASTRO_TEST_PASSWORD`). The account must be on the WorkOS CAPTCHA bypass allow list. In dev mode, `ASTRO_TEST_USERNAME` (your account handle) is also required.

### Running

Run via the Moon target (`scripts/smoke-test.sh`). It defaults to `ASTRO_ENV=dev` (local dev server at `http://localhost`, so `local-dev.sh` must be running):

```bash
# Against the local dev server (local-dev.sh running)
ASTRO_TEST_EMAIL=... ASTRO_TEST_PASSWORD=... ASTRO_TEST_USERNAME=... moon run tests:smoke

# Against prod
ASTRO_ENV=prod ASTRO_TEST_EMAIL=... ASTRO_TEST_PASSWORD=... moon run tests:smoke

# With the Playwright UI
ASTRO_TEST_EMAIL=... ASTRO_TEST_PASSWORD=... ASTRO_TEST_USERNAME=... moon run tests:smoke -- --ui
```

The suite config lives at `apps/tests/smoke/playwright.smoke.config.ts` (the authoritative project list).

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
| `app.secrets` | Variables & secrets, verifies auto-fill on deploy page | Session from setup | all |

If login fails, all projects that depend on `setup` are skipped automatically.

Tests run automatically every hour via the **"Smoke tests"** GitHub Actions workflow (`.github/workflows/smoke-test.yml`, `cron: "0 * * * *"`). They also run against preview after every deploy to `main`.

## Deployments

| Environment       | URL                   | Notes                                                  |
| ----------------- | --------------------- | ------------------------------------------------------ |
| Preview / Staging | https://astropod.ai   | Requires beta VPN                                      |
| Production        | https://astropods.com | Invite-only — contact a Postman team member for access |

**Preview** — Automatically deployed on every merge to `main`. No manual action required.

**Production** — Deployed manually. After changes land in `main` and are verified in preview, go to the **Actions** tab in GitHub, select the **"Deploy (Prod)"** workflow, and click **"Run workflow"**. You must select at least one service to deploy: `astro-server`, `astro-client`, or `astro-registry`.

To release the CLI to production, use the separate **"Release CLI (Prod)"** workflow.