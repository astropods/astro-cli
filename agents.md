# Project Overview

Astro is a platform for deploying and running AI agents of any kind. It provides agent-native infrastructure including models, knowledge bases, tool integrations, and observability. Agents can be packaged as containers and deployed with declarative configuration.

The project is a monorepo that contains packages for agent deployment infrastructure and optional utilities for building agents.

IMPORTANT: When planning and creating spec, please ensure it's concise and complete while trading off verbosity and grammar. Avoid putting code examples unless necessary.

# Apps

| App | Path | Purpose |
|-----|------|---------|
| astro-cli | `modules/astro-cli` | CLI for building, pushing, and deploying agents; handles local dev mode, container builds, registry push, and spec registration. Private git submodule (`astropods/astro-cli`); run `git submodule update --init modules/astro-cli` to work on it |
| astro-client | `apps/astro-client` | React web frontend for managing agents, deployments, observability, and team settings |
| astro-queen | `apps/astro-queen` | Admin console: a Cobra CLI that serves an embedded React SPA (`//go:embed web/dist`) backed by the AdminService gRPC API; provides access to cluster status, deployments, jobs, and observability |
| astro-registry | `apps/astro-registry` | Docker Registry V2 API proxy with auth; routes push/pull operations to backend registry (ECR) with membership checking |
| astro-server | `apps/astro-server` | Go backend API server handling agent registry, K8s deployments, auth (WorkOS), admin gRPC, and observability |
| astro-otel | `apps/astro-otel` | OTLP ingest service for local AI coding tools (e.g. Claude Code); authenticates account-scoped ingest keys against the DB and forwards traces→Langfuse and metrics→VictoriaMetrics |
| astro-proxy | `apps/astro-proxy` | Go reverse-proxy service for session-aware routing of requests to agents (`internal/{proxy,resolve,session}`) |
| tests | `apps/tests` | Playwright smoke/e2e suites; run via `moon run tests:smoke` (see repo README Smoke Tests) |

# Packages

| Package | Path | Purpose |
|---------|------|---------|
| astro-collector | `packages/astro-collector` | OpenTelemetry Collector distribution for collecting traces and metrics from deployed agents |
| astro-proto | `packages/astro-proto` | Protobuf definitions and generated code for gRPC services (AdminService API) |
| astro-spec | `packages/astro-spec` | YAML spec parser and types for `astropods.yml`; shared by CLI and server to parse/validate agent configuration. Public git submodule (`astropods/astro-spec`), consumed as Go module `github.com/astropods/astro-spec`; run `git submodule update --init packages/astro-spec` (keyless) to work on it |
| astro-brand-icons | `packages/astro-brand-icons` | Brand icon set with a build pipeline (`sources/` + `icons.json`) producing icon components |
| astro-identity-gen | `packages/astro-identity-gen` | Procedural, deterministic identity/avatar image generation |
| astro-theme | `packages/astro-theme` | Shared UI theme (design tokens and CSS) |
| astro-trading-card | `packages/astro-trading-card` | Agent trading-card rendering (SVG/PNG export, holo effects) |
| blueprint-jellybean | `packages/blueprint-jellybean` | Blueprint card rendering assets |

# Tooling

This is a bun monorepo. Always use `bun x <command>` instead of `npx`.

## Moon Task Cheatsheet

Use Moon as the default task runner from repo root.

- Discover tasks: `moon query tasks`
- Refresh this list: `moon query tasks`

### Current Moon Targets

<!-- This list can drift; regenerate with `moon query tasks`. -->


- `adapters:build`, `adapters:install-local`, `adapters:publish-local`
- `astro-brand-icons:clean`, `astro-brand-icons:dev`, `astro-brand-icons:process`, `astro-brand-icons:typecheck`
- `astro-cli:build`, `astro-cli:build-preview`, `astro-cli:clean`, `astro-cli:e2e`, `astro-cli:link`, `astro-cli:link-preview`, `astro-cli:test`, `astro-cli:typecheck`, `astro-cli:unlink`
- `astro-client:build-chat-embed`, `astro-client:clean`, `astro-client:dev`, `astro-client:e2e`, `astro-client:e2e.setup`, `astro-client:embed-into-cli`, `astro-client:lint`, `astro-client:test`, `astro-client:typecheck`
- `astro-collector:build`
- `astro-otel:build`, `astro-otel:fmt`, `astro-otel:lint`, `astro-otel:test`, `astro-otel:typecheck`, `astro-otel:vet`
- `astro-proto:deps`, `astro-proto:generate`, `astro-proto:typecheck`
- `astro-queen:build`, `astro-queen:deps`, `astro-queen:dev`, `astro-queen:fmt`, `astro-queen:link`, `astro-queen:start`, `astro-queen:typecheck`, `astro-queen:unlink`, `astro-queen:vet`, `astro-queen:web-build`, `astro-queen:web-install`
- `astro-registry:build`, `astro-registry:deps`, `astro-registry:fmt`, `astro-registry:lint`, `astro-registry:test`, `astro-registry:typecheck`, `astro-registry:vet`
- `astro-server:build`, `astro-server:deps`, `astro-server:dev`, `astro-server:e2e`, `astro-server:e2e.setup`, `astro-server:e2e.teardown`, `astro-server:fmt`, `astro-server:lint`, `astro-server:test`, `astro-server:test-integration`, `astro-server:typecheck`, `astro-server:vet`
- `astro-theme:build`, `astro-theme:clean`, `astro-theme:typecheck`
- `astro-trading-card:build`, `astro-trading-card:clean`, `astro-trading-card:typecheck`
- `deployment:astro-client`, `deployment:astro-registry`, `deployment:astro-server`, `deployment:clean`, `deployment:collector`, `deployment:messaging`, `deployment:smoke-test-astro-client`
- `messaging:proto-gen`, `messaging:publish-local`, `messaging:sdk-build`, `messaging:typecheck`
- `tests:smoke`

# Kubernetes API Usage

**K8s queries are for live status only, not for fetching data we already wrote.** When the server needs deployment/workload state (replica counts, pod phase, container readiness, restart counts, conditions), querying K8s is the only authoritative source — use it. But when the server already owns the data (env vars, spec, intent, URLs), read from the database instead.

Why: K8s API reads are not free. A single deployment-detail render that walks per-workload Secrets and ConfigMaps multiplies into dozens of GETs per poll per viewer; at scale that becomes a sustained load problem on the cluster, and during rolling updates the K8s view also lags the deployed spec until pods cycle. The DB is the apply-time intent — it doesn't lag, doesn't rate-limit, and we wrote it on purpose. Defaulting to K8s "because it's there" is exactly the trap to avoid.

Rule of thumb when adding a server-side read: if the question is *"what is the cluster doing right now?"* → K8s. If the question is *"what did we deploy?"* → DB. Don't conflate the two; the runtime endpoint and the record endpoint exist precisely to keep them separate (`apps/astro-server/handlers/deploy.go`).

# Data Fetching (astro-client)

All server data integration uses TanStack Query. See [docs/04-guides/tanstack-query.md](docs/04-guides/tanstack-query.md) for architecture, conventions, and best practices. Key rules:
- Never call `api.*` directly in components for reads — use query hooks from `src/api/queries/`.
- All query keys must come from the factories in `src/api/queries/keys.ts`.
- Mutations invalidate affected queries in `onSuccess`.

# Changelogs

Every PR must include a changelog file at `docs/changelog/{branch-name}-YYYY-MM-DD.md`. The filename must match the branch name exactly; for branches with slashes (e.g. `fix/my-change`), use subdirectories (e.g. `docs/changelog/fix/my-change-2026-03-10.md`). A GitHub Action warns on PRs missing one and auto-updates the PR description from it.

Changelogs must focus on **architecture and design**, not file-by-file diffs:
- **Summary** — The problem being solved and why the change exists.
- **Design** — How the pieces fit together, key decisions, with short code/config examples where helpful.
- **Migration** — What users need to do (or that nothing is required).

Do not list individual file changes. Explain the system, not the patch.

# Releases

Release notes live in `docs/releases/`. Each release is a single file named `YYYY-MM-DD.N.md` where `N` is a counter starting at `1` (e.g. `2026-04-10.1.md`, `2026-04-10.2.md`). Multiple releases per day are normal.

To create a new release:
1. Find the commit range: the previous release file lists its end commit; the new range starts there and ends at `HEAD`.
2. Read the relevant changelog files in `docs/changelog/` for the commits in range — they have the design context.
3. Write the release note with two sections:

**Public section** (top) — user-facing, no internal jargon:
- Feature headers describe what users can now do
- Bullet points describe observable behavior, not implementation
- No component names, API paths, DB details, or variable names
- Fixes section covers user-visible regressions only
- Migration table only if users need to take action

**Appendix** (bottom, separated by `---`) — internal details for the team:
- Commit range
- Technical specifics: component names, API params, env vars, DB/queue changes
- Fix root causes

4. Commit the release note file, then tag the commit:
```
git tag release/YYYY-MM-DD.N
```
The tag should match the filename exactly (e.g. `release/2026-04-10.2`).

# Product Documentation

Public product documentation lives in `docs-public/fern` (Fern). When writing or editing it, follow [docs-public/AGENTS.md](docs-public/AGENTS.md) — the source of truth for voice, structure, naming (always "Astro AI", never bare "Astro"), and what must not appear (internal rationale, component names, roadmap). See `docs-public/README.md` for local preview.