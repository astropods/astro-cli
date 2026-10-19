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

`astropods.yml` is the single source of truth for your agent. The `spec` field MUST be `blueprint/v1`:

```yaml
spec: blueprint/v1
name: my-agent
```

It declares:

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
  1  cd /path/to/project/hello-astropods    enter the project directory
  2  ast project configure                  set your model API keys
  3  ast project start                      start your agent locally
```

The wizard prompts for project name, description, LLM provider (Anthropic/OpenAI), interfaces (web, Slack), and optional knowledge or ingestion. Run `ast project configure` (or add during init if prompted) before starting dev.

### `project start` — Local development

Run your agent locally:

```bash
ast project start                  # start all containers
ast project start --skip-startup   # start without running startup jobs
ast project logs [service]         # tail agent logs (default)
ast project logs --all             # tail all service logs
ast project stop                   # stop and remove containers
ast project list                   # list local agents running on this machine
ast project trigger                # list the jobs in the spec
ast project trigger <name>         # run a job now and print its output
ast project trigger <name> --rebuild  # rebuild the job's container, then run it
```

In the foreground, `ast project start` tails the agent and every job that
runs while it is up, such as one started by `ast project trigger`.

Several projects can run at once. The first one gets the usual ports (chat UI
on http://localhost:3100). A project started while another holds a port gets a
free port instead; the ready block prints its real addresses, and
`ast project list` shows them for every running agent.

**Ingestion behavior:**

| Trigger type | What `ast project start` does |
|---|---|
| `startup` | Runs the ingestion container once at startup, unless `--skip-startup` is set |
| `schedule` | Prints `ast project trigger <name>` — run it manually on demand |
| `webhook` | Started alongside the agent; port exposed (default 3001) |
| `manual` | Prints `ast project trigger <name>` |

A `schedule` trigger has no cadence locally. Set the cron when you deploy:
`ast deploy <name> --schedule <job>="*/15 * * * *"`. The flag is
repeatable, takes a five-field cron expression, and is required for every
`schedule` entry on a first deploy. A redeploy keeps the cadence it already
has.

`ast deploy <name> --from-local` sends the values `ast project configure`
saved for the current directory. It sends each one as a plaintext value, not
a secret reference, so a saved API key reaches the deployment as its value.
To use an account secret for a key, also pass `--var KEY=@SECRET_NAME`. For
the same key, `--var` wins over `--vars-file`, which wins over `--from-local`.

### `push` — Build, package, and register with Astropods

`ast push` builds your project (if needed), packages the agent and spec, pushes images to a registry, and **adds the agent to the Astropods registry**. A build is run automatically unless you pass `--no-build`.

The push command requires an Astropods account. Use `ast login` to get the required credentials.

**Visibility:** On first push, the CLI prompts you to set the agent as **public** or **private**. Private agents are only visible to account members; public agents appear in the catalog and are accessible to anyone. Visibility is managed via the platform UI or API — `meta.visibility` in the spec is deprecated and no longer used.

**Existing blueprint names:** If the blueprint name already exists in the target account and you haven't pushed to it from this project on this machine before, the push stops with an error naming the account and blueprint. Push under a different name (change `name` in `astropods.yml`, or pass one on the command line), or pass `--yes` to push to it anyway.

**Git provenance and uncommitted changes:** For a normal push from a Git
repository, the CLI records the current commit SHA and message when available.
It checks the directory containing the selected spec and every declared
component `build.context`. Changes outside those paths do not affect the push,
but changes in a shared build context do.

If those inputs contain staged, unstaged, untracked nonignored, or submodule
changes, the push stops before building. Commit the changes or pass
`--allow-dirty` to continue and mark the pushed build as dirty. `--yes` skips
ordinary confirmation prompts but does not allow a dirty push.

Pass `-m`/`--message` to describe the push. The message replaces the commit
message on the build, including on a `--no-build` push.

Git discovery is best-effort. If the CLI records a commit but cannot verify
every relevant path, it warns that some build inputs may not be represented by
that commit and continues. Missing Git, a non-Git project, or a failed lookup
does not prevent a push. `--no-build` skips Git inspection and attaches no
commit information or dirty state because the current checkout cannot establish
the provenance of an image built earlier.

After pushing, **navigate to the Astropods platform** to see your agent and deploy it. As an agent builder/operator, you’ll see it in your **operator sandbox**. When you deploy an agent there, it receives a **dedicated hostname** that you can use to connect Slack or call as an API.

### `build` — Build containers (optional)

Build agent and custom component containers from the spec. This step is optional: when you run `ast push`, a build is done automatically if needed. Use `ast build` when you only want to build images (e.g. for local testing) without pushing.

### `github` — Act on GitHub pull requests and issues

`ast github` wraps the locally installed `gh` and `git` so an agent driving the CLI can read an issue, push a branch, open a pull request, and comment on it, without a separate GitHub client. This is unrelated to `ast push`, which registers a blueprint with Astropods rather than touching GitHub.

```bash
ast github issue view 42
ast github push --set-upstream
ast github pr create --title "Fix the thing" --body "Fixes the thing."
ast github pr comment 7 --body "Addressed the feedback."
ast github pr view 7 --json number,title,state
ast github pr checks 7
```

`pr`/`issue` commands run `gh`, so run `gh auth login` first; they use whatever account that already authenticated. `push` runs only `git`, so it uses git's own credentials (an SSH key, a credential helper, ...) and needs no `gh` login. `--repo OWNER/REPO` targets a repository other than the one in the current directory, the same as a plain `gh` invocation. Inside an `ast dev` session that shared a GitHub connection, every one of these commands uses that connection's token instead, so a sandboxed agent with no local git or `gh` credentials still works.
