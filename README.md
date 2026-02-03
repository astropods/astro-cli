# Astro CLI

A command-line interface for developing, building, and publishing AI agents on the Astro platform. Define your agent infrastructure declaratively in `astro.yml` and deploy with a single command.

## Features

- **Spec-driven development** - Declarative YAML configuration for agents, models, knowledge bases, tools, and integrations
- **Local development mode** - Run agents locally with hot reload for rapid iteration
- **Container orchestration** - Automatic Docker Compose generation for self-hosted components
- **OCI-native publishing** - Push agents and specs to any OCI-compatible registry
- **Multi-component builds** - Build agent containers plus custom models, knowledge stores, and tools
- **Message interface support** - Built-in Slack, Discord, and Teams messaging sidecars
- **Injection pipelines** - Schedule data ingestion from external sources with cron triggers

## Installation

### Build from source

```bash
moon run astro-cli:build
```

## Usage

### Commands

#### `dev` - Local Development

Run your agent locally with hot reload:

```bash
ast dev
```

This command:
- Loads environment variables from `.env`
- Builds and starts all self-hosted components (models, knowledge stores, tools)
- Runs your agent container with live code reloading
- Sets up messaging interfaces (Slack, Discord, Teams)
- Schedules injection workers based on cron triggers
- Watches `./src` for changes and auto-restarts

**Flags:**
- `--env` - Path to environment file (default: `.env`)
- `--no-reload` - Disable hot reload
- `--file, -f` - Path to spec file (default: `astro.yml`)

#### `build` - Build Containers

Build agent and custom component containers:

```bash
ast build
```

Builds:
- Agent container (if `container.build` is specified)
- Custom model containers (`models.*.container.build`)
- Custom knowledge store containers (`knowledge.*.container.build`)
- Custom tool containers (`tools.*.container.build`)

**Flags:**
- `--tag, -t` - Image tag (default: `latest`)
- `--no-cache` - Build without using cache
- `--file, -f` - Path to spec file (default: `astro.yml`)

#### `publish` - Publish to Registry

Publish agent, components, and spec to an OCI registry:

```bash
ast publish --registry ghcr.io/myorg
```

Publishes:
- Agent container image
- Custom component images
- Spec as OCI artifact (via ORAS)

**Flags:**
- `--registry, -r` - Registry URL (required)
- `--tag, -t` - Image tag (default: `latest`)
- `--build` - Build before publishing
- `--file, -f` - Path to spec file (default: `astro.yml`)

### Global Flags

- `--file, -f` - Path to astro.yml spec file (default: `astro.yml`)
- `--verbose, -v` - Enable verbose output
- `--quiet, -q` - Minimal output
- `--help, -h` - Show help

## Quick Start

1. Create an `astro.yml` spec file:

```yaml
spec: astro/v1
agent: my-agent

meta:
  version: 1.0.0
  description: My AI agent

container:
  build:
    context: .
    dockerfile: Dockerfile

integrations:
  models:
    llm:
      provider: anthropic

interfaces:
  slack:
    type: messaging/slack
    config:
      bot_token: ${SLACK_BOT_TOKEN}
      app_token: ${SLACK_APP_TOKEN}
```

2. Create a `.env` file with credentials:

```
ANTHROPIC_API_KEY=sk-...
SLACK_BOT_TOKEN=xoxb-...
SLACK_APP_TOKEN=xapp-...
```

3. Develop locally:

```bash
ast dev
```

4. Build containers:

```bash
ast build --tag v1.0.0
```

5. Publish to registry:

```bash
ast publish --registry ghcr.io/myorg --tag v1.0.0
```

## Spec Format

The `astro.yml` spec supports:

- **`meta`** - Version, description, tags, owner
- **`container`** - Agent runtime container configuration
- **`models`** - Self-hosted models (embeddings, inference)
- **`knowledge`** - Vector stores, key-value stores, graph databases
- **`tools`** - Custom functions and capabilities
- **`integrations`** - Cloud providers (Anthropic, OpenAI, GitHub, Tavily)
- **`interfaces`** - Messaging (Slack, Discord, Teams), HTTP APIs
- **`injections`** - Data pipelines with cron/event triggers

See example specs in `packages/astro-agents/` for reference.

## Development

### Prerequisites

- Go 1.24 or higher
- Docker and Docker Compose
- [moon](https://moonrepo.dev) - monorepo build tool

#### Installing moon

```bash
bash <(curl -fsSL https://moonrepo.dev/install/moon.sh)
export PATH="$HOME/.moon/bin:$PATH"
```

**Using npm/yarn/pnpm/bun:**
```bash
npm install --save-dev @moonrepo/cli
```

### Dependencies

- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Docker SDK](https://github.com/docker/docker) - Container builds and orchestration
- [ORAS](https://oras.land) - OCI artifact publishing
- [Compose Go](https://github.com/compose-spec/compose-go) - Docker Compose generation
- [fsnotify](https://github.com/fsnotify/fsnotify) - File watching for hot reload
- [cron](https://github.com/robfig/cron) - Cron scheduling for injections

### Project Structure

```
astro-cli/
├── main.go                     # Entry point
├── cmd/
│   ├── root.go                # Root command and global flags
│   ├── dev.go                 # Local development with hot reload
│   ├── build.go               # Container build orchestration
│   └── publish.go             # OCI registry publishing
├── internal/
│   ├── spec/
│   │   ├── parser.go          # YAML spec parsing
│   │   └── types.go           # Spec data structures
│   ├── compose/
│   │   └── builder.go         # Dynamic Compose project generation
│   └── watcher/
│       └── watcher.go         # File watcher for hot reload
├── go.mod
└── go.sum
```

## Architecture

The Astro CLI is a spec-driven orchestration tool that:

1. **Parses** declarative `astro.yml` specs into structured data
2. **Generates** Docker Compose projects dynamically based on declared components
3. **Builds** multi-stage container images for agents and custom components
4. **Orchestrates** local development environments with hot reload
5. **Publishes** containers and specs to OCI registries using ORAS

Key design principles:
- **Declarative over imperative** - Infrastructure as code via YAML specs
- **Container-native** - Everything runs in Docker for portability
- **OCI-compatible** - Works with any container registry
- **Developer-friendly** - Hot reload, credential injection, automatic service discovery
