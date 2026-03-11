# Astro CLI Design

CLI tool for creating, building, publishing, and developing AI agents.

---

## Commands

### astro create

Scaffold a new agent project with interactive configuration.

**Usage:**
```bash
astro create <agent-name>
```

**Options:**
- `--yes, -y` - Skip interactive mode, use defaults

**What it does:**
1. Launches an interactive TUI to configure the agent (or uses defaults with `--yes`)
2. Scaffolds a complete project directory with all necessary files

**Interactive TUI screens:**

| Screen | Options |
|--------|---------|
| Description | Free-text description for your agent |
| Interface | HTTP API, Slack |
| Model | Anthropic, OpenAI, None |
| Knowledge | Vector store, Key-Value store, Both, None |
| Tools | GitHub (multi-select) |
| Ingestion | Scheduled, Manual, Startup, None |

**Keyboard navigation:** `↑`/`↓` or `j`/`k` to move, `Space` to toggle (multi-select), `Enter` to confirm.

**Generated project structure:**
```
<agent-name>/
├── astropods.yml              # Agent specification
├── Dockerfile               # Agent container (Bun runtime)
├── package.json             # Bun dependencies
├── tsconfig.json            # TypeScript config
├── .env                     # Secrets and API keys
├── .gitignore
├── .dockerignore
├── agent/
│   └── index.ts             # Agent entry point
└── ingestion/               # One subdirectory per ingestion type (if enabled)
    └── <type>/
        ├── Dockerfile       # Ingestion container for this trigger type
        └── index.ts         # Ingestion script
```

**Naming rules:**
- Lowercase alphanumeric and hyphens only
- Must start with a letter, end with a letter or number
- Max 63 characters — pattern: `^[a-z][a-z0-9-]*[a-z0-9]$`
- Reserved names: `astro`, `agent`, `model`, `tool`

**Default configuration (with `--yes`):**

| Option | Default |
|--------|---------|
| Interface | HTTP |
| Model | Anthropic |
| Knowledge | None |
| Tools | None |
| Ingestion | Startup |

---

### astro build

Builds agent container and self-hosted component images from astropods.yml.

**Usage:**
```bash
astro build [options]
```

**Options:**
- `-f, --file <path>` - Path to astropods.yml (default: ./astropods.yml)
- `--no-cache` - Build without using cache
- `-t, --tag <tag>` - Tag for the agent image (default: latest)

**What it does:**
1. Validates astropods.yml spec
2. Builds agent container (from `container.build` or `container.image`)
3. Builds self-hosted component containers:
   - Models (from `models.*.container`)
   - Knowledge stores (from `knowledge.*.container`)
   - Tools (from `tools.*.container`)
4. Tags all images with consistent naming

**Output:**
- Agent image: `<agent-name>:tag`
- Component images: `<agent-name>-<component-name>:tag`

---

### astro publish

Publishes agent and components to OCI registry.

**Usage:**
```bash
astro publish [options]
```

**Options:**
- `-f, --file <path>` - Path to astropods.yml (default: ./astropods.yml)
- `-r, --registry <url>` - OCI registry URL (required)
- `-t, --tag <tag>` - Tag to publish (default: latest)
- `--build` - Build before publishing

**What it does:**
1. Pushes agent container to registry
2. Pushes self-hosted component containers to registry
3. Bundles astropods.yml spec as OCI artifact
4. Pushes spec artifact to registry (tagged with agent version from `meta.version`)
5. Creates manifest linking agent image + spec + components

**Output:**
- Registry reference: `<registry>/<agent-name>:<tag>`
- Spec artifact: `<registry>/<agent-name>/spec:<meta.version>`

---

### astro dev

Runs the agent and all supporting containers locally.

**Subcommands:**

| Subcommand | Description |
|---|---|
| `ast dev` / `ast dev start` | Build and start all containers (exits after start) |
| `ast dev logs [service]` | Tail container logs (default: agent; `--all` for all services) |
| `ast dev stop` | Stop and remove all containers |
| `ast dev trigger <name>` | Manually trigger a named ingestion job |

**Options (start):**
- `-f, --file <path>` - Path to astropods.yml (default: ./astropods.yml)
- `--env <file>` - Environment file for credentials (default: .env)
- `--rebuild` - Force rebuild without cache
- `--no-pull` - Skip pulling images

**What it does:**
1. Parses `astropods.yml`
2. Generates a Docker Compose project covering all spec components (models, knowledge, tools, messaging, ingestion)
3. Writes `.ast/docker-compose.yml`
4. Runs `docker compose up -d --build` and exits

**Ingestion handling:**

| Trigger type | Behaviour |
|---|---|
| `startup` | Ingestion container is run once synchronously before the CLI exits |
| `webhook` | Started alongside the agent as a persistent container; port exposed (default 3001) |
| `schedule` | Not auto-triggered; use `ast dev trigger <name>` on demand |
| `manual` | Not auto-triggered; use `ast dev trigger <name>` on demand |

**`ast dev trigger <name>`:**

Runs an ingestion container as a one-shot job against the running dev environment:
```bash
ast dev trigger schedule   # runs ingestion-schedule container and exits
```

**Integration credentials:**
- Use `ast configure` to set credentials (or `--env` to specify a file)
- Injects into all containers as environment variables

---

## Configuration

All commands read from astropods.yml in current directory by default.

**Global flags:**
- `--verbose, -v` - Verbose output
- `--quiet, -q` - Minimal output
- `--help, -h` - Show help

---

## Workflow Examples

**Create and develop:**
```bash
# Scaffold a new agent
astro create my-agent
cd my-agent
ast configure   # set API keys

# Start local development
astro dev
```

**Build and publish:**
```bash
# Build all images
astro build --tag v1.0.0

# Publish to registry
astro publish --registry ghcr.io/company --tag v1.0.0
```

**Quick iteration:**
```bash
# Build and publish in one step
astro publish --build --registry ghcr.io/company --tag latest
```

---

## Design Principles

1. **Convention over configuration** - Defaults work for 90% of cases
2. **Fast feedback** - Dev mode optimized for rapid iteration
3. **Reproducible builds** - Same spec = same images everywhere
4. **Registry-first** - Publish creates complete deployable artifact
5. **Local-first dev** - No cloud dependencies needed for development
