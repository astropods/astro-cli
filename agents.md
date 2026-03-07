# Project Overview

Astro is a platform for deploying and running AI agents of any kind. It provides agent-native infrastructure including models, knowledge bases, tool integrations, and observability. Agents can be packaged as containers and deployed with declarative configuration.

The project is a monorepo that contains packages for agent deployment infrastructure and optional utilities for building agents.

IMPORTANT: When planning and creating spec, please ensure it's concise and complete while trading off verbosity and grammar. Avoid putting code examples unless necessary.

# Apps

| App | Path | Purpose |
|-----|------|---------|
| astro-cli | `apps/astro-cli` | CLI for building, pushing, and deploying agents; handles local dev mode, container builds, registry push, and spec registration |
| astro-client | `apps/astro-client` | React web frontend for managing agents, deployments, observability, and team settings |
| astro-queen | `apps/astro-queen` | Bubbletea TUI admin client; provides interactive access to cluster status, deployments, and observability via gRPC |
| astro-registry | `apps/astro-registry` | Docker Registry V2 API proxy with auth; routes push/pull operations to backend registry (ECR) with membership checking |
| astro-server | `apps/astro-server` | Go backend API server handling agent registry, K8s deployments, auth (WorkOS), admin gRPC, and observability |

# Packages

| Package | Path | Purpose |
|---------|------|---------|
| astro-collector | `packages/astro-collector` | OpenTelemetry Collector distribution for collecting traces and metrics from deployed agents |
| astro-identity-gen | `packages/astro-identity-gen` | Procedural SVG avatar generator; deterministically produces unique visual identities from a seed string |
| astro-proto | `packages/astro-proto` | Protobuf definitions and generated code for gRPC services (AdminService API) |
| astro-spec | `packages/astro-spec` | YAML spec parser and types for `astropods.yml`; shared by CLI and server to parse/validate agent configuration |

# Tooling

This is a bun monorepo. Always use `bun x <command>` instead of `npx`.

# Data Fetching (astro-client)

All server data integration uses TanStack Query. See [docs/04-guides/tanstack-query.md](docs/04-guides/tanstack-query.md) for architecture, conventions, and best practices. Key rules:
- Never call `api.*` directly in components for reads — use query hooks from `src/api/queries/`.
- All query keys must come from the factories in `src/api/queries/keys.ts`.
- Mutations invalidate affected queries in `onSuccess`.

# Changelogs

Every PR must include a changelog file at `docs/changelog/{branch-name}-YYYY-MM-DD.md`. A GitHub Action warns on PRs missing one and auto-updates the PR description from it.

Changelogs must focus on **architecture and design**, not file-by-file diffs:
- **Summary** — The problem being solved and why the change exists.
- **Design** — How the pieces fit together, key decisions, with short code/config examples where helpful.
- **Migration** — What users need to do (or that nothing is required).

Do not list individual file changes. Explain the system, not the patch.