# Astro CLI

A command-line interface for developing, building, and pushing AI agents on the Astro platform. Define your agent infrastructure declaratively in `astropods.yml` and deploy with a single command.

## Features

- **Spec-driven development** — Declarative YAML configuration for agents, models, knowledge bases, tools, and integrations
- **Local development mode** — Run agents locally with hot reload for rapid iteration
- **Container orchestration** — Automatic Docker Compose generation for self-hosted components
- **Multi-component builds** — Build agent containers plus custom models, knowledge stores, and tools
- **Message interface support** — Built-in Slack sidecar
- **Ingestion pipelines** — Schedule data ingestion from external sources with cron triggers

## Quick Start

```bash
ast create my-astroid --yes # --yes skips all the prompts and uses default values
cd my-astroid
# If you didn't add your LLM API key during create, run ast configure
ast dev
```

This creates a starter project with all required assets (spec, Dockerfile, agent code, and config). Once `ast dev` is running, your agent is available locally; add interfaces (e.g. Slack) in the create wizard or by editing `astropods.yml` and running `ast configure`.

### Project structure

A typical project created by `ast create` looks like:

| Path | Purpose |
|------|--------|
| `astropods.yml` | Declarative spec: agent name, agent build, integrations (LLM, tools), interfaces (web, Slack), optional knowledge and ingestion. |
| `agent/` | Agent source code (e.g. `index.ts`). This is the main process that runs when you `ast dev`. |
| `ingestion/<type>/` | One folder per ingestion type (`schedule`, `webhook`, etc.), each with its own `Dockerfile` and `index.ts`. |
| `Dockerfile` | Builds the agent container. |
| `package.json`, `tsconfig.json` | Dependencies and TypeScript config for the agent (and ingestion, if present). |
| `.env` | Secrets and API keys (LLM, Slack, etc.). Use `ast configure` to set values before `ast dev`. |
| `postman/` | Postman collection for hitting the agent/API. |

Everything the CLI needs to build and run the agent is driven by `astropods.yml`; the rest of the tree is the code and config that the spec refers to.

### The `astropods.yml` file

`astropods.yml` is the single source of truth for your agent. It declares:

- **meta** — Description, tags, and visibility (`public` or `private`); used for push, display, and access control.
- **agent** — Agent runtime: build (e.g. `context: .`, `dockerfile: Dockerfile`) or a pre-built `image`.
- **models** (optional) — Self-hosted models.
- **knowledge** (optional) — Vector stores, KV stores, graph DBs; pre-built images (e.g. Qdrant, Redis) or custom builds.
- **integrations** — Services needed to run the agent. Credentials from `ast configure`.
- **interfaces** — How users talk to the agent: `web` (local UI), Slack, HTTP APIs. Config from `ast configure`.
- **ingestion** (optional) — Data pipelines with cron, manual, or startup triggers.

Edit this file to add/remove integrations, interfaces, or knowledge, and run `ast configure` for the matching credentials.

## Commands

### `create` — Scaffold a starter project

Create a new agent project with the required structure, `astropods.yml`, Dockerfile, and agent code. Run this first, then `cd` into the directory, run `ast configure` for credentials, and `ast dev`.

```bash
ast create hello-astropods -p /path/to/project
```

Sample output:

```
✓ Created agent hello-astropods

Next steps:
  → cd /path/to/project/hello-astropods
  → run ast configure for credentials
  → ast dev
```

The wizard prompts for project name, description, LLM provider (Anthropic/OpenAI), interfaces (web, Slack), and optional knowledge or ingestion. Run `ast configure` (or add during create if prompted) before starting dev.

### `dev` — Local development

Run your agent locally:

```bash
ast dev                        # start all containers
ast dev logs [service]         # tail logs (default: agent)
ast dev stop                   # stop and remove containers
ast dev trigger <name>         # manually trigger an ingestion job
```

**Ingestion behavior:**

| Trigger type | What `ast dev` does |
|---|---|
| `startup` | Runs the ingestion container once at startup |
| `schedule` | Prints `ast dev trigger <name>` — run it manually on demand |
| `webhook` | Started alongside the agent; port exposed (default 3001) |
| `manual` | Prints `ast dev trigger <name>` |

### `push` — Build, package, and register with Astro

`ast push` builds your project (if needed), packages the agent and spec, pushes images to a registry, and **adds the agent to the Astropods registry**. If images aren't already built, a build is run automatically unless you pass `--skip-build`.

The push command requires an Astro AI account. Use `ast login` to get the required credentials.

**Visibility:** On first push, the CLI prompts you to set the agent as **public** or **private**. You can also set `meta.visibility` in `astropods.yml` to skip the prompt. Private agents are only visible to account members; public agents appear in the catalog and are accessible to anyone. If you change `meta.visibility` in the spec after the first push, the CLI will ask you to confirm the change.

After pushing, **navigate to the Astro AI platform** to see your agent and deploy it. As an agent builder/operator, you'll see it in your **operator sandbox**. When you deploy an agent there, it receives a **dedicated hostname** that you can use to connect Slack, open in the Astro playground, or call as an API.

### `build` — Build containers (optional)

Build agent and custom component containers from the spec. This step is optional: when you run `ast push`, a build is done automatically if needed. Use `ast build` when you only want to build images (e.g. for local testing) without pushing.

### `playground` — Astropods playground

The **Astropods playground** is a local web UI to chat with your agent and try prompts. You can point it at a local agent (e.g. while running `ast dev`) or at a deployed agent’s hostname.

**Against a local agent** (messaging API on port 3100):

```bash
ast playground http://localhost:3100
```

This pulls the playground image (if needed), runs it, and opens the UI in your browser. You can override the local port with `--port` (default 3737) or use `--no-open` to avoid opening the browser.

**Against a deployed agent:** use the dedicated hostname that the Astro AI platform gives the agent after deployment, e.g. `ast playground https://example.agent.astropods.ai`.

The playground is useful for quick iteration during development and for testing a deployed agent before connecting Slack or other interfaces.
