# Astropods CLI

A command-line interface for developing, building, and pushing AI agents on the Astropods platform. Define your agent infrastructure declaratively in `astropods.yml` and deploy with a single command.

## Features

- **Spec-driven development** — Declarative YAML configuration for agents, models, knowledge bases, tools, and integrations
- **Local development mode** — Run agents locally with hot reload for rapid iteration
- **Container orchestration** — Automatic Docker Compose generation for self-hosted components
- **Multi-component builds** — Build agent containers plus custom models, knowledge stores, and tools
- **Message interface support** — Built-in Slack sidecar
- **Ingestion pipelines** — Schedule data ingestion from external sources with cron triggers

## Quick Start

```bash
ast project create my-astroid --yes # --yes skips all the prompts and uses default values
cd my-astroid
# If you didn't add your LLM API key during init, run ast project configure
ast project start
```

This creates a starter project with all required assets (spec, Dockerfile, agent code, and config). Once `ast project start` is running, your agent is available locally; add interfaces (e.g. Slack) in the create wizard or by editing `astropods.yml` and running `ast project configure`.

### Project structure

A typical project created by `ast project create` looks like:

| Path | Purpose |
|------|--------|
| `astropods.yml` | Declarative spec: agent name, agent build, integrations (LLM, tools), interfaces (web, Slack), optional knowledge and ingestion. |
| `agent/` | Agent source code (e.g. `index.ts`). This is the main process that runs when you `ast project start`. |
| `ingestion/<type>/` | One folder per ingestion type (`schedule`, `webhook`, etc.), each with its own `Dockerfile` and `index.ts`. |
| `Dockerfile` | Builds the agent container. |
| `package.json`, `tsconfig.json` | Dependencies and TypeScript config for the agent (and ingestion, if present). |
| `postman/` | Postman collection for hitting the agent/API. |

Everything the CLI needs to build and run the agent is driven by `astropods.yml`; the rest of the tree is the code and config that the spec refers to.

### The `astropods.yml` file

`astropods.yml` is the single source of truth for your agent. It declares:

- **agent** — Agent runtime: build (e.g. `context: .`, `dockerfile: Dockerfile`) or a pre-built `image`.
- **models** (optional) — Self-hosted models.
- **knowledge** (optional) — Vector stores, KV stores, graph DBs; pre-built images (e.g. Qdrant, Redis) or custom builds.
- **integrations** — Services needed to run the agent. Credentials from `ast project configure`.
- **interfaces** — How users talk to the agent: `web` (local UI), Slack, HTTP APIs. Config from `ast project configure`.
- **ingestion** (optional) — Data pipelines with cron, manual, or startup triggers.

Edit this file to add/remove integrations, interfaces, or knowledge, and run `ast project configure` for the matching credentials.

## Commands

### `project create` — Scaffold a starter project

Create a new agent project with the required structure, `astropods.yml`, Dockerfile, and agent code. Run this first, then `cd` into the directory, run `ast project configure` for credentials, and `ast project start`.

```bash
ast project create hello-astropods -p /path/to/project
```

Sample output:

```
✓ Created hello-astropods

Next steps:
  1  cd /path/to/project/hello-astropods   enter the project directory
  2  ast project configure                  set your API keys
  3  ast project start                      start your agent locally
```

The wizard prompts for project name, description, LLM provider (Anthropic/OpenAI), interfaces (web, Slack), and optional knowledge or ingestion. Run `ast project configure` (or add during init if prompted) before starting dev.

### `project start` — Local development

Run your agent locally:

```bash
ast project start                  # start all containers
ast project logs [service]         # tail agent logs (default)
ast project logs --all             # tail all service logs
ast project stop                   # stop and remove containers
ast project trigger <name>         # manually trigger an ingestion job
```

**Ingestion behavior:**

| Trigger type | What `ast project start` does |
|---|---|
| `startup` | Runs the ingestion container once at startup |
| `schedule` | Prints `ast project trigger <name>` — run it manually on demand |
| `webhook` | Started alongside the agent; port exposed (default 3001) |
| `manual` | Prints `ast project trigger <name>` |

### `push` — Build, package, and register with Astropods

`ast push` builds your project (if needed), packages the agent and spec, pushes images to a registry, and **adds the agent to the Astropods registry**. If images aren’t already built, a build is run automatically unless you pass `--skip-build`.

The push command requires an Astropods account. Use `ast login` to get the required credentials.

**Visibility:** On first push, the CLI prompts you to set the agent as **public** or **private**. You can also set `meta.visibility` in `astropods.yml` to skip the prompt. Private agents are only visible to account members; public agents appear in the catalog and are accessible to anyone. If you change `meta.visibility` in the spec after the first push, the CLI will ask you to confirm the change.

After pushing, **navigate to the Astropods platform** to see your agent and deploy it. As an agent builder/operator, you’ll see it in your **operator sandbox**. When you deploy an agent there, it receives a **dedicated hostname** that you can use to connect Slack or call as an API.

### `build` — Build containers (optional)

Build agent and custom component containers from the spec. This step is optional: when you run `ast push`, a build is done automatically if needed. Use `ast build` when you only want to build images (e.g. for local testing) without pushing.
