# CLI Command Tree

**Version:** 1.0
**Date:** 2026-04-20
**Status:** Proposal

## Abstract

The current CLI has a flat, inconsistent command structure that mixes local project operations (`create`, `configure`, `dev`) with platform operations (`push`, `knowledge`) and auth (`login`, `logout`, `whoami`). This spec defines a noun-verb command tree that groups commands by resource, establishes a working account context, and aligns the CLI surface to the full server API.

---

## 1. Motivation

- Commands are not consistently grouped: `ast create` scaffolds a project locally, `ast push` registers on the server, `ast knowledge create` manages a platform resource — no shared mental model.
- No concept of a working account context; every command implicitly uses a single account.
- Lifecycle operations for running deployments (stop, start, restart, rollback) have no CLI surface.
- Secret/variable management is absent.

---

## 2. Design Principles

- **Noun-verb**: every command is `ast <noun> <verb>`, e.g. `ast blueprint push`, `ast agent deploy`.
- **Org-scoped**: `ast account switch` sets the active account; all resource commands operate within that scope.
- **Local vs. platform distinction**: `project` commands are purely local; `blueprint` commands are purely server-side. They compose — `project init` scaffolds locally, `blueprint create` registers on the server, `blueprint push` builds and uploads.
- **Basic CRUD per resource**: each noun exposes `list`, `create`, `get`, `delete` (or `archive` where soft-delete is appropriate) plus resource-specific operations.

---

## 3. Command Tree

### `ast account`

| Command | Description |
|---|---|
| `account list` | List all accounts you belong to |
| `account switch <name>` | Set working account; scopes all subsequent commands |

The active account is persisted in the credential store alongside auth tokens. The credential store records all accounts the user belongs to and which is currently active. `account list` reads from the credential store; `account switch` updates the active entry.

---

### `ast blueprint`

Manages agent blueprints — the registered, versioned definitions of an agent on the platform.

| Command | Description |
|---|---|
| `blueprint list` | List blueprints in the active account |
| `blueprint create` | Register a new blueprint on the server |
| `blueprint get <name>` | Get blueprint metadata and config |
| `blueprint push` | Build image locally and push to registry |
| `blueprint archive <name>` | Archive a blueprint (soft delete) |
| `blueprint visibility <name> <public\|private>` | Set blueprint visibility |

`blueprint push` is the primary publish operation: it builds the container image, pushes it to the Astro registry, and registers the spec with the server. It subsumes the previous `ast push` command.

---

### `ast agent`

Manages running deployments — instances of a blueprint deployed to the platform.

| Command | Description |
|---|---|
| `agent list` | List deployments in the active account |
| `agent deploy` | Deploy a blueprint |
| `agent get <name>` | Get deployment status and detail |
| `agent delete <name>` | Undeploy |
| `agent stop <name>` | Scale to zero |
| `agent start <name>` | Wake up a stopped deployment |
| `agent restart <name>` | Rolling restart |
| `agent rollback <name>` | Roll back to previous revision |
| `agent history <name>` | List deployment history |
| `agent validate` | Dry-run a deploy spec without applying |
| `agent trigger <name> --ingestion <pipeline>` | Trigger a manual ingestion run |
| `agent logs <name>` | Fetch deployment logs; `--follow` streams live via SSE |

`agent validate` calls `POST /deploy/validate` and reports errors without creating or modifying a deployment.

`agent trigger` is scoped to a named running deployment; `--ingestion` names the pipeline defined in `astropods.yml`.

---

### `ast secrets`

Manages variables in the account vault. Values are write-only: they can be created, updated, and deleted, but never retrieved after being set.

| Command | Description |
|---|---|
| `secrets list` | List variables in the active account vault |
| `secrets create <name>` | Create a new variable |
| `secrets get <name>` | Get metadata for a variable (name, created/updated timestamps; value is never returned) |
| `secrets update <name>` | Update an existing variable's value |
| `secrets delete <name>` | Delete a variable |
| `secrets import <file>` | Bulk-create or update variables from a `.env`-formatted file |

---

### `ast project`

Local project operations. No network calls except `project dev` (which manages local containers).

| Command | Description |
|---|---|
| `project init` | Scaffold a new local agent project |
| `project configure` | Interactively set credentials and input values |
| `project dev` | Manage local dev environment (start / stop / logs subcommands) |
| `project build` | Build agent container image locally |
| `project validate` | Validate `astropods.yml` against the spec schema |
| `project add` | Add a model, tool, or knowledge store to the spec |
| `project explain` | Print a plain-language summary of the spec |

`project init` replaces `ast create`. `project build` makes the build step explicit and separable from push.

---

### Auth and CLI management

| Command | Description |
|---|---|
| `login` | Authenticate with Astro (device flow) |
| `logout` | Clear stored credentials |
| `whoami` | Show the currently authenticated user |
| `upgrade` | Upgrade the CLI to the latest version |
| `version` | Print CLI version |

---

## 4. Global Flags

| Flag | Description |
|---|---|
| `--file, -f <path>` | Path to `astropods.yml` (default: `./astropods.yml`) |
| `--verbose, -v` | Verbose output |
| `--quiet, -q` | Minimal output |

---

## 5. Migration from Current Commands

| Old command | New command |
|---|---|
| `ast create` | `ast project init` |
| `ast push` | `ast blueprint push` |
| `ast configure` | `ast project configure` |
| `ast dev` | `ast project dev` |
| `ast validate` | `ast project validate` |
| `ast add` | `ast project add` |
| `ast explain` | `ast project explain` |
| `ast knowledge *` | unchanged for now |

---

## 6. Open Questions

- **`agent trigger` shape**: the `--ingestion <pipeline>` flag assumes pipeline names are known at call time. If a deployment has only one pipeline, should `--ingestion` be optional and default to the only available pipeline?
