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
├── astroai.yml              # Agent specification
├── Dockerfile             # Agent container (Bun runtime)
├── Dockerfile.ingestion   # Ingestion pipeline (if enabled)
├── package.json           # Bun dependencies
├── tsconfig.json          # TypeScript config
├── .env.example           # Environment variables template
├── .gitignore
├── .dockerignore
├── agent/
│   └── index.ts           # Agent entry point (HTTP server)
└── ingestion/
    └── index.ts           # Data pipeline script
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

Builds agent container and self-hosted component images from astroai.yml.

**Usage:**
```bash
astro build [options]
```

**Options:**
- `-f, --file <path>` - Path to astroai.yml (default: ./astroai.yml)
- `--no-cache` - Build without using cache
- `-t, --tag <tag>` - Tag for the agent image (default: latest)

**What it does:**
1. Validates astroai.yml spec
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
- `-f, --file <path>` - Path to astroai.yml (default: ./astroai.yml)
- `-r, --registry <url>` - OCI registry URL (required)
- `-t, --tag <tag>` - Tag to publish (default: latest)
- `--build` - Build before publishing

**What it does:**
1. Pushes agent container to registry
2. Pushes self-hosted component containers to registry
3. Bundles astroai.yml spec as OCI artifact
4. Pushes spec artifact to registry (tagged with agent version from `meta.version`)
5. Creates manifest linking agent image + spec + components

**Output:**
- Registry reference: `<registry>/<agent-name>:<tag>`
- Spec artifact: `<registry>/<agent-name>/spec:<meta.version>`

---

### astro dev

Runs agent locally with hot reload for development.

**Usage:**
```bash
astro dev [options]
```

**Options:**
- `-f, --file <path>` - Path to astroai.yml (default: ./astroai.yml)
- `--env <file>` - Environment file for integration credentials (default: .env)
- `--no-reload` - Disable hot reload

**What it does:**
1. Validates astroai.yml spec
2. Spins up self-hosted components locally:
   - Models (docker containers from `models.*.container`)
   - Knowledge stores (docker containers from `knowledge.*.container`)
   - Tools (docker containers from `tools.*.container`)
3. Builds and runs agent container with:
   - Volume mounts for hot reload (watches source files)
   - Injected connection strings for local components
   - Injected credentials from .env for integrations
   - Port forwarding for interfaces
4. Watches for changes:
   - Rebuilds agent on source code changes
   - Restarts agent container
   - Does NOT rebuild component containers (restart dev to rebuild)

**Integration credentials:**
- Reads from .env file (or specified --env)
- Expected format:
  ```
  ANTHROPIC_API_KEY=sk-...
  GITHUB_TOKEN=ghp_...
  PINECONE_API_KEY=...
  ```
- Injects into agent container as environment variables

**Hot reload:**
- Watches: `container.build.context` directory
- Ignores: node_modules, .git, build artifacts
- On change: rebuilds agent image, restarts container
- Components stay running (manual restart needed for component changes)

---

## Configuration

All commands read from astroai.yml in current directory by default.

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
cp .env.example .env

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
