# CLI Command Tree

**Version:** 1.1
**Date:** 2026-04-25
**Status:** Partially implemented — `account`, `blueprint`, `secrets` are live; `agent` and `project` are planned

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
| `account token` | Print an API token scoped to the active account |

The active account is persisted in the credential store alongside auth tokens. The credential store records all accounts the user belongs to and which is currently active. `account list` reads from the credential store; `account switch` updates the active entry.

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
| `blueprint validate` | Validate `astropods.yml` against the spec schema |
| `blueprint set <name>` | Update blueprint settings (e.g. `--visibility public\|private`) |
| `blueprint archive <name>` | Archive a blueprint (soft delete) |

Top-level aliases for the most common operations are registered at the root: `ast build <name>`, `ast push <name>`, and `ast validate` delegate to the corresponding `blueprint` subcommands.

`blueprint push` is the primary publish operation: it validates the spec, optionally builds the container image, pushes it to the Astro registry, and registers the spec with the server. The agent name is a required positional argument; if it differs from the spec's `name` field, the CLI warns before proceeding. Pass `--build` to build the image before pushing.

`blueprint validate` runs full schema and semantic validation without authenticating or touching the registry. The cobra handlers for `push` and `build` both run validation before auth/build machinery — a bad spec exits immediately with a clear error.

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
| `--verbose, -v` | Verbose output |
| `--quiet, -q` | Minimal output |

`-f/--file <path>` is **not** a global flag. It is scoped to the individual commands that accept a spec file: `blueprint build`, `blueprint push`, `blueprint validate`, and their top-level aliases. Commands that don't operate on a local spec do not advertise it.

---

## 5. Migration from Current Commands

| Old command | New command | Status |
|---|---|---|
| `ast push` | `ast blueprint push <name>` (alias: `ast push <name>`) | **Implemented** — top-level alias preserved |
| `ast build` | `ast blueprint build <name>` (alias: `ast build <name>`) | **Implemented** — top-level alias preserved |
| `ast validate` | `ast blueprint validate` (alias: `ast validate`) | **Implemented** |
| `ast create` | `ast project init` | Planned |
| `ast configure` | `ast project configure` | Planned |
| `ast dev` | `ast project dev` | Planned |
| `ast add` | `ast project add` | Planned |
| `ast explain` | `ast project explain` | Planned |
| `ast knowledge *` | unchanged for now | — |

Flags removed from `ast push`: `--skip-build`, `--skip-push`, `--skip-register`, `--no-auth`, `--server`, `--registry`, `--platform`. The platform is fixed to `linux/amd64` for production pushes.

---

## 6. Open Questions

- **`agent trigger` shape**: the `--ingestion <pipeline>` flag assumes pipeline names are known at call time. If a deployment has only one pipeline, should `--ingestion` be optional and default to the only available pipeline?
