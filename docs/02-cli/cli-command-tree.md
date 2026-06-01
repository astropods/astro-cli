# CLI Command Tree

**Version:** 1.2
**Date:** 2026-04-26
**Status:** Implemented — all sections below reflect the current state of the CLI

## Abstract

The CLI uses a noun-verb command tree that groups commands by resource, establishes a working account context, and aligns the CLI surface to the full server API. Local project operations live under `project`; platform operations live under `blueprint`, `agent`, and `secrets`.

---

## 1. Design Principles

- **Noun-verb**: every command is `ast <noun> <verb>`, e.g. `ast blueprint push`, `ast agent deploy`.
- **Org-scoped**: `ast account switch` sets the active account; all resource commands operate within that scope.
- **Local vs. platform distinction**: `project` commands are purely local; `blueprint` commands are purely server-side. They compose — `project create` scaffolds locally, `blueprint push` builds and uploads.
- **Top-level aliases**: the most common operations (`build`, `push`, `deploy`, `create`) are available at the root as convenience shortcuts.

---

## 2. Command Tree

### `ast account`

| Command | Description |
|---|---|
| `account list` | List all accounts you belong to |
| `account switch [name]` | Set working account; scopes all subsequent commands. With no name, opens an interactive picker (esc to quit without changing). Use `-` to switch back to the previous account. |
| `account token` | Print an API token scoped to the active account |

The active account is persisted in the credential store alongside auth tokens.

---

### `ast blueprint`

Manages agent blueprints — the registered, versioned definitions of an agent on the platform.

| Command | Description |
|---|---|
| `blueprint list` | List blueprints in the active account |
| `blueprint create <name>` | Register a new blueprint on the server (hidden; `push` auto-creates) |
| `blueprint get <name>` | Get blueprint metadata and config |
| `blueprint build <name>` | Build agent container image locally |
| `blueprint push <name>` | Build (optional) and push image to registry; auto-creates blueprint |
| `blueprint deploy <name>` | Deploy a blueprint to the platform |
| `blueprint set <name>` | Update blueprint settings (e.g. `--visibility public\|private`) |
| `blueprint archive <name>` | Archive a blueprint (soft delete) |

Top-level aliases: `ast build <name>`, `ast push <name>`, `ast deploy <name>`.

**Deploy / redeploy flags** (shared by `blueprint deploy`, `deploy`, and `agent redeploy`): `--adapter`, `--var`, `--vars-file`, `--build`, `--dry-run`, `--json`. Blueprint deploy also supports `--name` / `-n` for display name.

---

### `ast agent`

Manages running deployments — instances of a blueprint deployed to the platform.

Target a deployment with exactly one of `--name` (display name or blueprint name from `agent list`) or `--id` (deployment ID). IDs are not accepted on `--name`. Quote names that contain spaces or shell metacharacters: `ast agent get --name 'Pirate Parrot EU!'`.

| Command | Description |
|---|---|
| `agent list` | List deployments in the active account |
| `agent get` | Get deployment status and detail (`--name` or `--id`) |
| `agent delete` | Undeploy (`--name` or `--id`, `--confirm`) |
| `agent pause` | Scale to zero (`--name` or `--id`) |
| `agent resume` | Wake up a paused deployment (`--name` or `--id`) |
| `agent restart` | Rolling restart (`--name` or `--id`, `--component`) |
| `agent redeploy` | Redeploy with updated config or build (`--name` or `--id`) |
| `agent history` | List deployment history (`--name` or `--id`) |
| `agent logs` | Fetch deployment logs; `--tail` streams live (`--name` or `--id`, `--workload` accepts `workload[/container]`) |
| `agent trace` | List traces or show a single trace Overview (`--name` or `--id`, `--trace-id` for detail) |

---

### `ast secrets`

Manages variables in the account vault. Values are write-only: they can be created, updated, and deleted, but never retrieved after being set.

| Command | Description |
|---|---|
| `secrets list` | List variables in the active account vault |
| `secrets create <name>` | Create a new variable |
| `secrets get <name>` | Get metadata for a variable (name, timestamps; value is never returned) |
| `secrets update <name>` | Update an existing variable's value |
| `secrets delete <name>` | Delete a variable |
| `secrets import <file>` | Bulk-create or update variables from a `.env`-formatted file |

---

### `ast project`

Local project operations. Alias: `dev`. No network calls except `project start` (which manages local containers via Docker Compose).

| Command | Description |
|---|---|
| `project create [name]` | Scaffold a new local agent project |
| `project configure` | Set credentials and input values (interactive TUI or flags) |
| `project start` | Build and start the local dev environment |
| `project stop` | Stop and remove local containers |
| `project logs [service]` | Tail container logs |
| `project trigger <name>` | Manually trigger a named ingestion pipeline |

Top-level alias: `ast create [name]` → `ast project create`.

---

### `ast spec`

Validate and explain `astropods.yml` spec files.

| Command | Description |
|---|---|
| `spec validate` | Validate `astropods.yml` against the spec schema |
| `spec explain` | Print a plain-language summary of the spec |
| `spec repair` | Detect and update outdated generated files |

---

### `ast settings`

Manage CLI settings and shell completions.

| Command | Description |
|---|---|
| `settings update` | Enable or disable anonymous usage telemetry |
| `settings bash` | Write bash completion script to `~/.ast/` and print install instructions |
| `settings zsh` | Write zsh completion script to `~/.ast/` and print install instructions |
| `settings fish` | Write fish completion script to `~/.ast/` and print install instructions |
| `settings powershell` | Write PowerShell completion script to `~/.ast/` and print install instructions |

---

### Auth and CLI management

| Command | Description |
|---|---|
| `login` | Authenticate with Astropods (device flow) |
| `logout` | Clear stored credentials |
| `whoami` | Show the currently authenticated user |
| `upgrade` | Upgrade the CLI to the latest version |
| `docs [category]` | Display Astropods documentation in the terminal |
| `add` | Add a model, tool, or knowledge store to the spec |
| `knowledge` | Manage managed knowledge stores |
| `connect` | Connect this device to Astropods |

---

## 3. Global Flags

| Flag | Description |
|---|---|
| `--verbose, -v` | Verbose HTTP and operation output |

`-f/--file <path>` is not a global flag. It is a persistent flag on `spec` (inherited by its subcommands) and registered independently on `blueprint build` and `blueprint push`.

---

## 4. Migration from Previous Commands

| Old command | New command | Notes |
|---|---|---|
| `ast push` | `ast blueprint push <name>` | Top-level `ast push <name>` alias preserved |
| `ast build` | `ast blueprint build <name>` | Top-level `ast build <name>` alias preserved |
| `ast deploy` | `ast blueprint deploy <name>` | Top-level `ast deploy <name>` alias preserved |
| `ast create` | `ast project create` | Top-level `ast create` alias preserved |
| `ast dev` | `ast project start` | `ast project` has alias `dev`; `ast dev start` still works |
| `ast configure` | `ast project configure` | `set`/`unset` subcommands replaced by `--var`/`--rm-var` flags |
| `ast explain` | `ast spec explain` | |
| `ast validate` | `ast spec validate` | |
| `ast configure --no-telemetry` | `ast settings update --telemetry off` | |
| `ast configure set KEY VALUE` | `ast project configure --var KEY=VALUE` | Empty value (`KEY=`) now stores `""` rather than deleting |
| `ast configure unset KEY` | `ast project configure --rm-var KEY` | |
| `ast knowledge *` | unchanged | |
