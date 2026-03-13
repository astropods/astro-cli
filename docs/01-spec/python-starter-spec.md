# Python Starter Specification

**Version:** 1.0
**Date:** 2026-03-12
**Status:** Draft

## Abstract

This spec defines the requirements for adding Python as a first-class language in the Astro CLI scaffolding system. It covers four deliverables: a Python messaging package (`astropods-messaging`) containing generated gRPC stubs, a Python core adapter package (`astropods-adapter-core`) that ports the TypeScript bridge to Python, a LangChain adapter package (`astropods-adapter-langchain`) that bridges LangChain agents to the core, and CLI changes that generate a working Python agent project from `ast create --lang py`. LangChain was chosen as the default framework because it is the most widely adopted Python agent framework, mirrors the single-agent simplicity of the Mastra starter, supports all major LLM providers natively, and has a streaming interface that maps cleanly to the Astro adapter pattern. The two-package structure mirrors the TypeScript architecture and enables future Python framework adapters (CrewAI, LangGraph, etc.) to depend only on the core package.

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 1. Introduction

The Python starter occupies three layers of the Astro developer experience:

```
Python Adapter Packages  →  CLI Template Assets  →  Generated Agent Project
```

The adapter packages are the runtime dependency layer. `astropods-adapter-core` is a Python port of `@astropods/adapter-core` that provides the `AgentAdapter` protocol, `MessagingBridge` gRPC client, and `serve()` entry point. `astropods-adapter-langchain` depends on `astropods-adapter-core` and provides the `LangChainAdapter` implementation, mirroring how `@astropods/adapter-mastra` depends on `@astropods/adapter-core`. The CLI template assets are the project scaffolding layer: embedded files rendered by `ast create --lang py` into a working project directory. The generated project installs both adapter packages and calls `serve()` to connect to the messaging sidecar.

Implementation proceeds in three phases:

1. **Phase 1: Python Adapter Packages.** All three packages are developed and published to PyPI. They can be tested independently against a running messaging sidecar before any CLI changes are made.
2. **Phase 2: CLI Flag and Template Assets.** Template files are added for Python, and `ast create --lang py` is wired up. The interactive TUI form is unchanged in this phase.
3. **Phase 3: Language Selection in TUI.** The interactive form is updated to include a language selection step, so users who run `ast create` without flags are prompted to choose between TypeScript and Python.

### Scope

This spec covers the requirements, interface contracts, and publishing pipeline for all deliverables. PyPI account setup and package ownership configuration are out of scope.

---

## 2. System Overview

### 2.1 Component Layout

```
┌─────────────────────────────────────────────────────────┐
│  ast create --lang py                                   │
│  ┌───────────────────────────────────────────────────┐  │
│  │  template-py/          (shared Python files)      │  │
│  │  ├── Dockerfile                                   │  │
│  │  ├── Dockerfile.ingestion                         │  │
│  │  ├── astropods.yml     (reused from template-ts/) │  │
│  │  ├── gitignore.tmpl                               │  │
│  │  ├── dockerignore.tmpl                            │  │
│  │  ├── agents.md.tmpl                               │  │
│  │  ├── README.md.tmpl                               │  │
│  │  └── postman/collections/                         │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │  template-py-langchain/  (LangChain-specific)     │  │
│  │  ├── agent/main.py     (agent entry point)        │  │
│  │  └── requirements.txt                             │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
         │
         │ generated project imports
         ▼
┌─────────────────────────────────────────────────────────┐
│  astropods-adapter-langchain  (PyPI)                    │
│  └── LangChainAdapter                                   │
│       │ depends on                                      │
│       ▼                                                 │
│  astropods-adapter-core  (PyPI)                         │
│  ├── AgentAdapter protocol                              │
│  ├── MessagingBridge (gRPC client)                      │
│  ├── serve() entry point                                │
│  └── depends on astropods-messaging (gRPC stubs)        │
│                                                         │
│  Connects to GRPC_SERVER_ADDR ──► Messaging Sidecar     │
└─────────────────────────────────────────────────────────┘
```

### 2.2 TypeScript Parallel

The Python adapter mirrors the TypeScript adapter architecture exactly:

| TypeScript                  | Python                        | Role                                                  |
| --------------------------- | ----------------------------- | ----------------------------------------------------- |
| `@astropods/messaging`      | `astropods-messaging`         | Generated gRPC stubs                                  |
| `@astropods/adapter-core`   | `astropods-adapter-core`      | `AgentAdapter` protocol, `MessagingBridge`, `serve()` |
| `@astropods/adapter-mastra` | `astropods-adapter-langchain` | Framework-specific adapter                            |

### 2.3 Package Structure

The three new packages live alongside the existing TypeScript packages in the adapters submodule:

```
modules/adapters/packages/
├── core/           (TypeScript, unchanged)
├── mastra/         (TypeScript, unchanged)
├── messaging-py/   NEW: Python gRPC stubs (mirrors @astropods/messaging)
│   ├── pyproject.toml
│   └── src/
│       └── astropods_messaging/
│           ├── __init__.py
│           ├── audio_pb2.py
│           ├── audio_pb2_grpc.py
│           ├── config_pb2.py
│           ├── config_pb2_grpc.py
│           ├── feedback_pb2.py
│           ├── feedback_pb2_grpc.py
│           ├── message_pb2.py
│           ├── message_pb2_grpc.py
│           ├── response_pb2.py
│           ├── response_pb2_grpc.py
│           └── service_pb2_grpc.py
├── core-py/        NEW: Python port of core/
│   ├── pyproject.toml
│   └── src/
│       └── astropods_adapter_core/
│           ├── __init__.py
│           ├── types.py       (AgentAdapter protocol, StreamHooks, StreamOptions)
│           ├── bridge.py      (MessagingBridge gRPC client)
│           └── serve.py       (serve() entry point)
└── langchain/      NEW: Python LangChain adapter (mirrors mastra/)
    ├── pyproject.toml
    └── src/
        └── astropods_adapter_langchain/
            ├── __init__.py
            └── adapter.py     (LangChainAdapter)
```

---

## 3. Python Adapter Packages

### 3.1 Messaging Package (`astropods-messaging`)

The messaging package MUST:

- Contain Python gRPC stubs generated from the proto files in `@astropods/messaging`
- Be published to PyPI as `astropods-messaging`
- Declare `grpcio` and `protobuf` as required dependencies

Stubs MUST be generated with `grpcio-tools` from the proto source files and committed to the package. They MUST NOT be generated at install time.

### 3.2 Core Package (`astropods-adapter-core`)

The core package MUST:

- Provide an `AgentAdapter` protocol that any Python agent adapter implements
- Connect to the messaging sidecar at `GRPC_SERVER_ADDR` via the `ProcessConversation` bidirectional gRPC stream
- Retry the connection with exponential backoff on startup
- Receive incoming messages from the sidecar and dispatch them to the agent
- Translate agent output into the gRPC messaging protocol (content chunks, status updates, errors)
- Expose a `serve()` entry point that agent projects call to start the bridge
- Be published to PyPI as `astropods-adapter-core`

It MUST declare `astropods-messaging` as a required dependency. It MUST NOT declare any agent framework as a dependency.

### 3.3 AgentAdapter Interface

The `AgentAdapter` protocol defines the contract between the messaging bridge and any agent adapter implementation. It MUST declare:

| Member           | Description                                                          |
| ---------------- | -------------------------------------------------------------------- |
| `name`           | Display name for the agent, used in logs and registration            |
| `stream()`       | Async method that streams a response for a given prompt              |
| `get_config()`   | Returns agent metadata (system prompt, tools) for playground display |

### 3.4 StreamHooks

The `stream()` method receives a `hooks` object. Implementations MUST call these hooks as the agent produces output:

| Hook                       | When to call                                                     |
| -------------------------- | ---------------------------------------------------------------- |
| `on_chunk(text)`           | Each token or text fragment from the LLM                         |
| `on_status_update(status)` | Agent state changes (thinking, tool use, etc.)                   |
| `on_error(error)`          | An error occurred during generation                              |
| `on_finish()`              | Response is complete. MUST be called exactly once per request    |

Valid status values: `THINKING`, `SEARCHING`, `GENERATING`, `PROCESSING`, `ANALYZING`, `CUSTOM`.

### 3.5 MessagingBridge

The `MessagingBridge` connects to the messaging sidecar and routes messages between the sidecar and the agent adapter. It is a Python port of `messaging-bridge.ts` in `modules/adapters/packages/core/`. It MUST implement the same retry, registration, and streaming behavior as the TypeScript implementation.

### 3.6 LangChain Package (`astropods-adapter-langchain`)

The LangChain package MUST:

- Declare `astropods-adapter-core` and `langchain` as required dependencies
- Provide a `LangChainAdapter` that wraps a LangChain `AgentExecutor` and implements the `AgentAdapter` protocol
- Translate LangChain's streaming events into the appropriate `StreamHooks` calls, including mapping tool lifecycle events to status updates
- Be published to PyPI as `astropods-adapter-langchain`

---

## 4. CLI Template Assets

Template assets are files embedded in the `ast` binary and rendered into the user's project directory by `ast create`.

### 4.1 Directory Structure

```
apps/astro-cli/internal/scaffold/templates/
├── template-ts/              (existing, unchanged)
├── template-ts-mastra/       (existing, unchanged)
├── template-py/              NEW: shared Python files
│   ├── Dockerfile
│   ├── Dockerfile.ingestion
│   ├── gitignore.tmpl
│   ├── dockerignore.tmpl
│   ├── agents.md.tmpl
│   ├── README.md.tmpl
│   └── postman/
│       └── collections/
│           ├── messaging.postman_collection.json
│           └── webhook.postman_collection.json
└── template-py-langchain/    NEW: LangChain-specific files
    ├── agent/
    │   └── main.py
    ├── ingestion/
    │   ├── main.py
    │   └── webhook.py
    └── requirements.txt
```

### 4.2 Shared Files (`template-py/`)

The `astropods.yml` template is reused from `template-ts/` since the Astro spec is language-agnostic. The Dockerfile MUST use a multi-stage build and run as a non-root user.

### 4.3 LangChain-Specific Files (`template-py-langchain/`)

`agent/main.py` MUST be a Go template that conditionally imports the correct LangChain provider based on the selected integration and calls `serve()` as its final statement.

`requirements.txt` MUST be a Go template that conditionally includes provider-specific LangChain packages based on the selected integrations, and always includes `astropods-adapter-langchain`.

### 4.4 Ingestion Pipelines

`ingestion/main.py` MUST be a Go template that mirrors `ingestion/index.ts` — a standalone script with environment variable comments and placeholder patterns for each selected knowledge store. `ingestion/webhook.py` MUST be a Go template that mirrors `ingestion/webhook.ts` — a simple HTTP server with a `/webhook` POST endpoint and placeholder ingestion logic. Both files MUST be skipped when no ingestion triggers are selected in `ScaffoldConfig`.

---

## 5. CLI Code Changes

### 5.1 Language Support (Phase 2)

The CLI `create` command MUST support `--lang py` in addition to the existing `--lang ts`. When `--lang py` is provided without an explicit `--template`, the template MUST default to `langchain`.

### 5.2 Language-Aware Scaffold Generation (Phase 2)

Scaffold file generation MUST be language-aware. TypeScript-only files (`tsconfig.json`, `.npmrc`, `package.json`) MUST be skipped when `lang=py`. Python-only files (`requirements.txt`, `agent/main.py`, `ingestion/main.py`, `ingestion/webhook.py`) MUST be skipped when `lang=ts`. The messaging Postman collection MUST be generated for both languages. The webhook Postman collection MUST be generated for Python when webhook ingestion is selected.

### 5.3 Language Selection in TUI (Phase 3)

The interactive form MUST include a language selection step before the agent name prompt. Options: TypeScript (Bun) and Python. The selected language determines which template is used and replaces the need to pass `--lang` explicitly.

---

## 6. Publishing Pipeline

The publish workflow at `modules/adapters/.github/workflows/publish.yml` MUST be updated to publish the three new Python packages to PyPI as part of Phase 1. The workflow MUST:

- Build and publish `astropods-messaging`, `astropods-adapter-core`, and `astropods-adapter-langchain` in dependency order
- Trigger on the same manual `workflow_dispatch` event as the existing npm publish steps
- Use PyPI trusted publishing (OIDC) rather than a stored API token

---

## 7. Validation Rules

Implementations MUST enforce the following:

1. `lang` MUST be one of `ts` or `py`. Any other value MUST return an error before scaffold generation begins.
2. `template` MUST be compatible with the selected `lang`. `mastra` is only valid with `ts`; `langchain` is only valid with `py`. Mismatched combinations MUST return an error.
3. When `lang=py`, `ingestion/main.py` MUST be generated when any ingestion trigger is selected. `ingestion/webhook.py` MUST only be generated when the webhook ingestion trigger is selected.
4. `on_finish()` MUST be called exactly once per request. It MUST NOT be called if `on_error()` has already been called for the same request.
5. `on_chunk()` and `on_status_update()` MUST NOT be called after `on_finish()` or `on_error()` has been called for the same request.
6. gRPC stubs in `astropods-messaging` MUST NOT be edited by hand. They MUST be regenerated via `grpcio-tools` when proto files in `@astropods/messaging` change.

---

## Appendix A: Non-Goals (Non-Normative)

- **Multiple Python framework starters**: Only LangChain for the initial CLI template. The two-package adapter structure enables additional framework adapters (e.g. `astropods-adapter-crewai`) and CLI templates to be added later without changes to the core package.
- **Working ingestion logic**: Ingestion templates (`ingestion/main.py`, `ingestion/webhook.py`) are stubs with TODO placeholders, matching the TypeScript templates. Actual embedding and knowledge store logic is left to the user.
- **Voice/audio support**: The Python adapter implements text streaming only. Audio support is a future addition.
- **Python dev mode hot-reload**: File-watching dev tooling is out of scope for this phase.
