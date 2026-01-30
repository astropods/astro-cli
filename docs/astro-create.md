# astro create

Scaffold a new Astro agent project with interactive configuration.

## Usage

```bash
astro create <agent-name>
```

## Overview

The `create` command launches an interactive TUI to configure your agent, then scaffolds a complete project directory with all necessary files. Use the `--yes` flag to skip the TUI and use defaults.

## Flags

| Flag | Description |
|------|-------------|
| `--yes, -y` | Skip interactive mode, use defaults |

## Interactive Configuration

The TUI guides you through the following screens:

| Screen | Options |
|--------|---------|
| Description | Free-text description for your agent |
| Interface | HTTP API, Slack |
| Model | Anthropic, OpenAI, None |
| Knowledge | Vector store, Key-Value store, Both, None |
| Tools | GitHub (multi-select) |
| Ingestion | Scheduled, Manual, Startup, None |

**Keyboard navigation:**
- `↑`/`↓` or `j`/`k` - Move selection
- `Space` - Toggle option (multi-select screens)
- `Enter` - Confirm and proceed

## Generated Project Structure

```
<agent-name>/
├── astro.yml              # Agent specification
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

## Naming Rules

Agent names must follow these constraints:

- Lowercase alphanumeric characters and hyphens only
- Must start with a letter
- Must end with a letter or number
- Maximum 63 characters
- Pattern: `^[a-z][a-z0-9-]*[a-z0-9]$`

**Reserved names:** `astro`, `agent`, `model`, `tool`

## Examples

```bash
# Interactive mode - launches TUI
astro create my-agent

# Non-interactive with defaults
astro create my-agent --yes
```

## Default Configuration

When using `--yes`, the following defaults are applied:

| Option | Default |
|--------|---------|
| Interface | HTTP |
| Model | Anthropic |
| Knowledge | None |
| Tools | None |
| Ingestion | Startup |

## Next Steps

After creating your project:

1. Navigate to the project directory:
   ```bash
   cd <agent-name>
   ```

2. Copy the environment template and add credentials:
   ```bash
   cp .env.example .env
   ```

3. Start local development:
   ```bash
   astro dev
   ```
