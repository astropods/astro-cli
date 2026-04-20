# Learnings from Memory Box: What Astropods Should Become

Memory Box is a personal knowledge management agent built on the Astropods platform by an AI agent (Claude Code). It stores text, URLs, images, PDFs, and files, makes them searchable via hybrid semantic + keyword search, and exposes a React dashboard, REST API, and MCP server — all from a single container.

This document is a retrospective framed around a single question: **what did the agent expect to be able to do, and where did reality — through missing capability or missing documentation — get in the way?** Every item in §4 is a concrete expectation the agent formed from the spec, the schema, or consistency with other sections, and what happened when that expectation hit the platform.

Each gap is tagged by its type:

- **CAPABILITY** — the platform doesn't do this yet; needs code.
- **DOCS** — the platform does do it, but nothing tells you so; the agent had to read Go source to find out.
- **DESIGN** — the platform deliberately doesn't do this; the expectation itself should be corrected (by better docs, clearer spec, or a pointer to the right mechanism).

The split matters: CAPABILITY gaps need engineering, DOCS gaps are cheap to close but cause the most frustration per line of missing text, and DESIGN gaps are usually signals that the spec is accepting a field it shouldn't or naming something ambiguously.

---

## Table of Contents

1. [What Memory Box Built](#1-what-memory-box-built)
2. [How It Evolved](#2-how-it-evolved)
3. [What Worked Well](#3-what-worked-well)
4. [Expectation Gaps (What Broke or Required Workarounds)](#4-expectation-gaps-what-broke-or-required-workarounds)
5. [What a New Developer Should Be Able to Assume](#5-what-a-new-developer-should-be-able-to-assume)
6. [Missing Primitives That Would Generalize](#6-missing-primitives-that-would-generalize)
7. [Spec Syntax Recommendations](#7-spec-syntax-recommendations)
8. [Engineering Fixes (Specific)](#8-engineering-fixes-specific)
9. [Summary of Recommendations](#9-summary-of-recommendations)

---

## 1. What Memory Box Built

**Final architecture**: A single Hono HTTP server (Bun, port 80) serving:

- React SPA dashboard (Vite + Tailwind)
- REST API (session-authenticated for dashboard, bearer-token for ingestion)
- MCP server (Model Context Protocol for Claude integration)
- Mastra agent with 7 tools (store, search, get, list, delete, graph-query, display)
- Background job system (GitHub import, Twitter import, reprocessing)
- OAuth 2.1 server (Dynamic Client Registration, PKCE)

**Infrastructure dependencies**:

- PostgreSQL + pgvector (metadata, embeddings, auth, sessions, jobs)
- MinIO (file/image/PDF storage)
- Nomic Embed v1.5 (embedding model, self-hosted)
- Anthropic Claude (agent reasoning)

**Scale of the project**: ~40 source files, 7 agent tools, 9 database tables, 4-stage ingestion pipeline (detect, classify, extract, chunk/embed/store), hybrid search (vector + fulltext + reciprocal rank fusion).

---

## 2. How It Evolved

The project's git history tells a story of progressive simplification driven by operational reality:

### Phase 1: Ambitious distributed architecture

Started with Qdrant (vectors) + Redis (state) + Neo4j (graph) + separate ingestion webhook + messaging sidecar + Mastra adapter. Five containers, three databases, multiple communication channels.

### Phase 2: Database consolidation

Replaced Qdrant/Redis/Neo4j with a single PostgreSQL + pgvector instance. One database handles metadata, embeddings, fulltext search, auth tokens, sessions, jobs, and settings. This was the biggest win — eliminated coordination complexity between three databases with no capability loss.

### Phase 3: Container consolidation

Merged the separate ingestion webhook, agent container, and dashboard into a single HTTP server using `interfaces.frontend: true`. Deleted `@astropods/adapter-mastra` dependency entirely. The agent is now just a library, not a separate process.

### Phase 4: Chat UI iteration

Tried assistant-ui (off-the-shelf chat component library), hit limitations with streaming animation, replaced it with custom SSE streaming + Streamdown. Pattern: tried the generic solution, hit edge cases, built bespoke when the generic tool didn't fit.

### Phase 5: Refinement

MCP server, OAuth, mobile responsiveness, background jobs, import systems. This is where the agent spent most of its time — building features the platform didn't provide.

**Key insight**: The project converged toward fewer, simpler components over time. Every architectural simplification was driven by deployment friction or operational complexity. The platform should learn from this trajectory.

---

## 3. What Worked Well

### The spec format itself

Declarative YAML for defining an agent's infrastructure is the right abstraction. Saying "I need a Postgres database" and having it appear is powerful. The `provider` concept for built-in services (Qdrant, Redis, Postgres, Neo4j, Ollama) is well-designed.

### `ast dev` for local development

Generating Docker Compose from the spec and running everything locally is a strong workflow. Being able to iterate without touching cloud infrastructure matters.

### Built-in provider conventions

For built-in providers, env var injection (`POSTGRES_HOST`, `QDRANT_PORT`, etc.) and health checks work seamlessly. The agent code just reads the env var and connects.

### `interfaces.frontend: true`

Serving a web UI from the agent container is the right default for agents that have their own dashboard. It simplifies deployment by eliminating a separate frontend service.

### Model provider declarations

`models.anthropic.provider: anthropic` is clean. The platform handles API key injection, and the agent code just uses the model. The separation between cloud providers (API key) and self-hosted providers (container) is intuitive.

---

## 4. Expectation Gaps (What Broke or Required Workarounds)

> Each item below is a real expectation the agent formed — usually by reading another part of the spec and assuming consistency — and what happened when that expectation hit the code. Gap type tags: **CAPABILITY** (missing code), **DOCS** (works, but you have to read the source to know it), **DESIGN** (deliberately not supported).
>
> Some issues from the original feedback have since shipped on main; each item is annotated with its current status.

### 4.1. Custom knowledge containers get no env var injection in `ast dev` — CAPABILITY (HIGH — OPEN)

**Expected**: Declaring a custom container for knowledge should inject `HOST`/`PORT` env vars into the agent, the same way a provider-backed entry does. Both declare a sidecar container; both need to be reachable from the agent.

**Reality**: In `ast dev`, the agent gets nothing. No `HOST`, no `PORT`, no `URL`. The compose only starts the sidecar — the agent has no way to discover it.

**Root cause**: `builder.go:BuildEnvironment()` (lines 733–762) only injects `{EnvPrefix}_HOST/PORT` when the knowledge entry has a provider with a non-empty `EnvPrefix`. For a custom container (no provider), `knowledge.Provider == ""`, so `GetProvider("")` returns a zero-value and the `if prov.EnvPrefix != ""` guard fails silently. The whole branch is skipped.

**The deployment path gets this right.** `envresolver.go:resolveKnowledgeConnections` (lines 581–635) falls through to container-mode wiring and emits `KNOWLEDGE_{UPPER(name)}_{HOST|PORT}`. So the same spec works in production and fails in `ast dev` — the worst kind of gap: the one you only hit after you've shipped.

**Workaround still needed**: Hardcoded Docker Compose service names in application code:

```typescript
const host = process.env.POSTGRES_HOST || process.env.KNOWLEDGE_DB_HOST || 'knowledge-db';
```

**Fix**: Add the §8.3 fallback to `BuildEnvironment()`, matching the envresolver logic:

```go
// After the provider EnvPrefix block (line 762):
if prov.EnvPrefix == "" && knowledge.Container != nil {
    prefix := "KNOWLEDGE_" + spec.SanitizeEnvName(name)
    env[prefix+"_HOST"] = &serviceName
    port := fmt.Sprintf("%d", container.Port)
    env[prefix+"_PORT"] = &port
}
```

### 4.2. `container.Environment` is a silent trap for knowledge/integration/ingestion — DESIGN + DOCS (MEDIUM — OPEN)

**Expected**: `environment: { POSTGRES_PASSWORD: secret }` on a container is accepted by the schema and works for model containers. The agent assumed it would work for knowledge/integration/ingestion containers too — Docker Compose familiarity reinforces this.

**Reality**: It validates. It silently does nothing. Values never reach the container.

**What happened on main**: Commit `7f59c742` briefly added `container.Environment` passthrough for knowledge containers. Thirteen minutes later, commit `b34c226c` **deliberately replaced it** with the `inputs` mechanism:

> Replace the environment passthrough approach with proper inputs injection [...] This is the correct mechanism: inputs are surfaced in the deploy UI, support secret storage, and are prompted via `ast configure`.

**Current state**: `inputs` on knowledge/integration/ingestion entries correctly inject into their own containers during `ast dev` (builder.go lines 412–467 for knowledge, 454–467 for integrations). This is the intended mechanism. But `container.Environment` still validates at the schema level for these sections, and models still apply it (builder.go lines 174–181) — so the inconsistency reads as "sometimes this field works, sometimes it doesn't, and nothing tells you which."

**Gap type**: The design decision is sound (`inputs` is the right primitive). The failure is that the schema still accepts `environment` for sections where it's dead, and no docs warn you. This is a DOCS gap bolted onto a DESIGN decision.

**Recommendation**: See §8.6. Either validate-and-warn, or drop `environment` from the schema for non-model sections. Don't make it work everywhere — that reverses the team's stated direction.

### 4.3. Custom container persistence fails at runtime — FIXED

**Original problem**: `persistent: true` on a custom container created a volume but didn't know where to mount it.

**Status**: Fixed in commit `fea6a941`. The `volume` field was added to `ContainerConfig`, the JSON schema was regenerated (`0adfe6ab`), and `builder.go` (lines 368-375) now uses `container.Volume` with provider fallback and a clear error message:

```
knowledge "db": persistent is true but no volume path specified (set container.volume)
```

### 4.4. Startup dependency ordering is weak — CAPABILITY (MEDIUM — OPEN)

**Expected**: "My agent starts after its dependencies are ready." The agent declared a Postgres knowledge store; it assumed `ast dev` would sequence startup so the agent only launches once Postgres is accepting queries. This is standard Docker Compose — health checks plus `depends_on: condition: service_healthy`.

**Reality**: The agent starts as soon as the Postgres process launches, often before it can accept connections. First query returns `connection refused` or `database system is starting up`.

**Two things fail together:**

1. **`depends_on` condition is wrong.** The agent now depends on all other services via `depends_on` (builder.go lines 607–614), but uses `ServiceConditionStarted` (process running) not `ServiceConditionHealthy` (accepting connections).

2. **Knowledge containers have no health check.** Model containers (builder.go lines 212–219) fall back to the provider's `HealthPath`/`HealthCheck` when none is explicitly set. Knowledge containers (lines 346–365) don't — they only add a health check when `container.Healthcheck != nil`. And `ResolvedContainer()` for knowledge (spec.go lines 193–206) doesn't copy the provider's health check; it only picks up `Image`, `Port`, and `Persistent`.

**Net effect**: Even for the built-in `postgres` provider — which registers `pg_isready` in the provider registry — the knowledge container in `ast dev` has no health check, and the agent starts before Postgres is ready.

**Workaround still needed**: Application-level retry loops:

```typescript
for (let attempt = 1; attempt <= 15; attempt++) {
  try { await pool.query(schema); return; }
  catch (err) { await new Promise(r => setTimeout(r, 2000)); }
}
```

**Fix (two parts)**:
1. In knowledge container setup, fall back to provider health check (matching model behavior):
   ```go
   if container.Healthcheck == nil && knowledge.IsProviderMode() {
       prov := spec.GetProvider(knowledge.Provider)
       if len(prov.HealthCheck) > 0 {
           container.Healthcheck = &spec.Healthcheck{Test: prov.HealthCheck}
       }
   }
   ```
2. Change `depends_on` condition to `ServiceConditionHealthy` for services that have health checks.

### 4.5. Knowledge credentials don't propagate to the agent — DESIGN + DOCS (MEDIUM — OPEN)

**Expected**: Setting `POSTGRES_PASSWORD` as an `input` on the knowledge entry injects it into the knowledge container AND makes it available to the agent — because the agent is the one connecting to it. If only the container has the secret, the agent can't use it.

**Reality**: `inputs` on a knowledge entry inject into the knowledge container only. The agent gets nothing. To connect, the agent must either hardcode the credential or duplicate the input at the top level.

**Why it works this way**: Spec §8.4 scopes component inputs to their own container to avoid leaking secrets across containers. This is a legitimate security design decision — you don't want a generic tool input bleeding into every sidecar. The problem is there's no way to say "this credential is part of the connection string; give it to whoever needs to connect."

**Gap type**: The design is defensible, but the consequence — that the agent can't reach its own declared knowledge store without a workaround — isn't surfaced anywhere. This is a DOCS gap on top of a DESIGN decision that needs a proper escape hatch.

**Workaround**: Hardcode credentials in application code, or duplicate them as top-level inputs.

**Recommendation**: Inject a complete connection string (`KNOWLEDGE_{NAME}_URL`) that embeds credentials from the component's inputs. This gives the agent what it needs without exposing individual credential values to unrelated containers. See [§6.1](#61-connection-string-injection).

### 4.6. No first-class HTTP service / MCP support — CAPABILITY (MEDIUM — OPEN)

**Expected**: An agent that serves a REST API, an MCP endpoint, and a React SPA should be able to say so in the spec. Each of these is a distinct kind of interface — the platform could route, TLS-terminate, and assign public URLs differently for each.

**Reality**: The `Interfaces` struct only has `Frontend` and `Messaging` booleans (spec.go lines 43–46). No `services`, no `mcp`, no general HTTP exposure. Note: commit `bb634fd4` added `interfaces.auth.web` for OIDC opt-in, but at the **deployment template** level in astro-server — not at the agent spec level. From the agent's perspective, the top-level interfaces surface is unchanged.

**Workaround**: Overload `frontend: true` to serve REST, MCP, and the SPA from a single Hono server. It works, but the spec can't express what the container actually does, and the platform can't reason about it — can't generate MCP client configs, can't scope auth per interface, can't split routing.

### 4.7. No hot-reload dev workflow for frontend agents — CAPABILITY (LOW — OPEN)

**Expected**: `ast dev` iterates fast. Change a file, see the result. This is what `ast dev` vs. deploy fundamentally means.

**Reality**: With `frontend: true`, `ast dev` runs the built container. No hot reload. Real iteration requires running three processes manually: the agent container for databases, a local Vite server for the SPA, and a local Bun server for the API, with CORS and localhost overrides wiring them together.

**What exists**: The agent gets a bind mount for the `agent/` directory (builder.go lines 589–596), and `dev.command` can override the start command. Together these cover the common case of a single-entry Bun agent.

**Why the workaround is needed**: Real projects have code outside `agent/` — `server/`, `lib/`, `tools/`, `dashboard/`. The bind mount is too narrow, so changes in those directories don't trigger a reload.

**Workaround**: Separate dev scripts with CORS and localhost overrides; three terminals open during development.

### 4.8. `ast docs` doesn't cover the sharp edges — DOCS (LOW — OPEN)

**Expected**: `ast docs` is how a new agent-builder learns the platform. It should explain every concept that `ast init` or the schema exposes.

**Reality**: Several load-bearing concepts are absent:

- Custom knowledge containers (when to use `container` vs `provider`)
- Persistence (`volume` semantics, provider vs. custom mount path)
- Env var naming conventions (`{EnvPrefix}_HOST` vs `KNOWLEDGE_{NAME}_HOST`)
- `environment` vs `inputs` and when each applies
- Which provider gets which default env (`POSTGRES_DB` is auto-derived; nothing says so)

**Consequence**: every one of the gaps in §4.1–4.7 was discovered the hard way. The agent read Go source to understand behavior. A newcomer without Go comfort would be stuck.

**Gap type**: Pure DOCS. Most of the missing content is mechanical — the schema, the provider registry, and the env resolver already contain the truth; `ast docs` just doesn't project it.

---

## 5. What a New Developer Should Be Able to Assume

If someone with no knowledge of Astropods wanted to build an agent like Memory Box, these things should "just work":

### 5.1. "I declare a database, my agent can connect to it"

This is the #

1 expectation. Writing this in the spec:

```yaml
knowledge:
  db:
    container:
      image: pgvector/pgvector:pg17
      port: 5432
```

Should mean the agent container automatically receives `DB_HOST`, `DB_PORT`, and `DB_URL` (or `KNOWLEDGE_DB_HOST`, etc.) — without any manual wiring, hardcoded hostnames, or knowledge of Docker Compose internals.

### 5.2. "I can configure my sidecar containers through the spec"

Writing `environment: { POSTGRES_PASSWORD: secret }` on a container should pass that env var to the container. The schema accepts it, so it should work.

### 5.3. "Persistent means persistent"

`persistent: true` should make data survive restarts, regardless of whether the container is a built-in provider or a custom image. The platform should either infer the mount path from the image's VOLUME declaration or accept an explicit `volume` field.

### 5.4. "My agent starts after its dependencies are ready"

The agent shouldn't need retry loops. If it declares knowledge stores, those stores should be healthy before the agent starts. This is standard Docker Compose `depends_on` with health checks.

### 5.5. "I can serve HTTP endpoints"

Agents that serve web UIs, APIs, or protocol endpoints (MCP, webhooks) should have a first-class way to declare this. The platform should handle routing, TLS, and public URL assignment.

### 5.6. "Connection strings are available, not just host/port"

Most database clients want a connection string, not separate host/port/user/password variables. The platform should inject a ready-to-use URL:

```
KNOWLEDGE_DB_URL=postgresql://postgres:postgres@knowledge-db:5432/memory_box
```

### 5.7. "I can see what env vars my agent will receive"

An `ast dev env` command (or equivalent) that prints all injected environment variables would eliminate guesswork.

---

## 6. Missing Primitives That Would Generalize

These aren't Memory Box-specific — they apply to any agent being built on the platform.

### 6.1. Connection string injection

Every agent with a database goes through the same dance: read HOST, read PORT, construct connection string, handle missing vars. The platform knows all the pieces — it should assemble the string.

**Proposed**: For every knowledge entry, inject `{NAME}_URL` with the full connection string, including credentials from inputs.

### 6.2. Explicit dependency declaration

Memory Box discovered that the spec implicitly assumes the agent can reach all knowledge services but provides no mechanism for:

- Declaring which services a container depends on
- Startup ordering
- Scoping network access in production

**Proposed**: An explicit `uses` field on the agent:

```yaml
agent:
  uses:
    - knowledge.db
    - knowledge.files
```

This tells the platform: inject connection info, set up depends_on, generate connection strings, scope network access.

### 6.3. Provider + custom image hybrid

The project discovered a useful middle ground: using a built-in provider's conventions (env prefix, mount path, health check) with a custom image. This pattern should be first-class:

```yaml
knowledge:
  db:
    provider: postgres               # use postgres conventions
    container:
      image: pgvector/pgvector:pg17  # but with this image
    persistent: true
```

This already works (accidentally) because `ResolvedContainer()` picks up the user's image while `BuildEnvironment()` uses the provider's env prefix. But it's not documented and feels like exploiting an implementation detail.

### 6.4. Guidance on schema initialization / migrations

Memory Box runs `db-schema.sql` on startup via application code. Many agents will need to initialize their database schema. A one-shot `init:` file in the spec is tempting but a footgun — it has no answer for versioned migrations, idempotence, or schema evolution once the agent is past v1.

**Better options, in order of cost**:

1. **Document the pattern.** Pick a recommended migration approach (e.g., Postgres migrations run at agent startup, idempotent, versioned) and describe it in `ast docs`. Most agents will then do the right thing without any platform change.
2. **Startup-hook primitive.** If the platform adds anything, make it a pre-start hook on the agent itself (run a command before the agent's main process) rather than an `init:` on the knowledge entry. That generalizes to non-DB initialization too and doesn't pretend schema versioning is a platform concern.

The thing to avoid is a single-file `init:` primitive that implies platform-managed schema.

### 6.5. File/object storage as a first-class provider

Memory Box needed S3-compatible storage (MinIO) for files, images, and PDFs. This is a common need — many agents handle binary content. Adding an `s3` or `files` built-in provider would save every project from rolling its own MinIO setup:

```yaml
knowledge:
  files:
    provider: s3           # platform provides MinIO
    persistent: true
    # injects FILES_HOST, FILES_PORT, FILES_URL, FILES_ACCESS_KEY, FILES_SECRET_KEY
```

### 6.6. Embedding model as a first-class provider

Memory Box runs Nomic Embed v1.5 as a custom container. Embedding is a foundational capability for RAG agents. A built-in provider would save setup:

```yaml
models:
  embeddings:
    provider: embeddings   # platform provides a text embedding model
    models: [nomic-embed-text]
```

**Caveat**: embedding model choice matters more than the spec shorthand implies — dimensions, domain, and licensing differ meaningfully across Nomic, BGE, Jina, and OpenAI-compatible endpoints. The right shape is probably a provider with a sensible default model plus explicit override, not a black-box "embeddings" provider.

### 6.7. Background jobs / cron

Memory Box built its own job system (job registry, state tracking, concurrency control, cron scheduler) because the platform has no equivalent.

**The right direction is to extend what already exists, not invent a parallel primitive.** The `ingestion` section with `trigger: schedule` is the closest thing and is architecturally consistent with "agents as containers." In-process `handler:` references tied to a file path inside the agent container would be runtime-specific (Bun-only, Python-only, etc.) and would blur the container boundary that makes the platform portable.

Concrete improvements to the existing primitive that would have removed Memory Box's custom job system:

- Lightweight scheduled ingestion entries that share the agent image (no separate Dockerfile), invoked as a subcommand or HTTP webhook on the agent
- A platform-managed run log (state, last run, last error) exposed to the dashboard
- Concurrency controls (`max_concurrent: 1`) on the schedule, so overlapping runs don't step on each other

The goal is the same — periodic syncing, reprocessing, cleanup — without committing the platform to being a generic in-process cron runner.

### 6.8. MCP server interface

MCP (Model Context Protocol) is becoming the standard way for AI tools to expose capabilities. Memory Box had to build its own MCP server from scratch. A platform-level primitive would help:

```yaml
agent:
  interfaces:
    frontend: true
    mcp:
      path: /mcp           # or separate port
      auth: bearer          # platform manages token generation
```

The platform could handle session management, auth, health checks, and generate connection configs for MCP clients.

### 6.9. Auth / secrets management

Memory Box built its own auth system (bearer tokens, sessions, bcrypt passwords, OAuth 2.1). Many agents need some form of authentication for their APIs.

**What already exists**: Commit `bb634fd4` added `interfaces.auth.web` as a **deployment-template** opt-in for OIDC on the messaging web ingress. That solves "protect the web UI with your org's IdP" for the messaging surface.

**What's still missing — at the agent level**:

- API token generation and validation primitives the agent code can call
- Session management (cookie-based dashboard sessions are re-built in every agent)
- Secret injection that reaches both the knowledge container and the agent (see §6.1)
- OAuth 2.1 / Dynamic Client Registration for MCP clients — Memory Box built this from scratch

The pattern: `interfaces.auth.web` protects the ingress; the agent's own API endpoints and MCP servers still need a token/session story the platform doesn't currently provide. These complement rather than conflict.

---

## 7. Spec Syntax Recommendations

Based on Memory Box's final state and what it wished it could write, here's what the ideal spec could look like for a project of this complexity:

### What Memory Box has today

```yaml
spec: package/v1
name: memory-box
meta:
  visibility: public
agent:
  build:
    context: .
    dockerfile: Dockerfile
  interfaces:
    frontend: true
    messaging: false
models:
  anthropic:
    provider: anthropic
knowledge:
  db:
    provider: postgres
    persistent: true
    # POSTGRES_DB is auto-derived from the agent name (SanitizeDBName)
  files:
    container:
      build:
        context: .
        dockerfile: minio.Dockerfile
      port: 9000
      volume: /data
    persistent: true
  embeddings:
    container:
      image: mindthemath/nomic-embed-v1.5:slim
      port: 8080
dev:
  interfaces:
    frontend:
      port: 3001
```

### What it would look like with the proposed improvements

```yaml
spec: package/v1
name: memory-box

agent:
  build:
    context: .
    dockerfile: Dockerfile
  interfaces:
    frontend: true
    mcp:
      path: /mcp
  uses:
    - knowledge.db
    - knowledge.files
    - knowledge.embeddings

models:
  anthropic:
    provider: anthropic

knowledge:
  db:
    provider: postgres
    image: pgvector/pgvector:pg17    # custom image, postgres conventions
    persistent: true
  files:
    provider: s3                     # built-in MinIO provider
    persistent: true
  embeddings:
    provider: embeddings
    models: [nomic-embed-text]

dev:
  interfaces:
    frontend:
      port: 3001
      hot_reload: true
```

**What changed** (annotated with current reality):

- No `messaging: false` — omitting defaults to false when frontend is set
- No `inputs` for standard database config — _partly already shipped: `POSTGRES_DB` auto-derives from the agent name via `spec.SanitizeDBName`_
- No custom Dockerfiles for MinIO or embeddings — built-in providers (§6.5, §6.6)
- `image` shorthand on provider entries instead of nested `container.image` — makes the provider+custom-image hybrid (§6.3) first-class
- Explicit `uses` for dependency declaration — enables connection-string injection (§6.1) and network scoping
- `mcp` as a recognized interface (§6.8)
- `hot_reload` for dev workflow (§4.7)

---

## 8. Engineering Fixes (Specific)

Updated to reflect the current state of main. Some original fixes have shipped; the remaining ones are small and high-impact.

### 8.1. ~~Support `volume` on custom containers~~ — SHIPPED

Fixed in `fea6a941`. The `Volume` field was added to `ContainerConfig`, schema regenerated, and `builder.go` uses `container.Volume` with provider fallback and clear error messaging.

### 8.2. ~~Inject component inputs into sidecar containers~~ — SHIPPED

Fixed in `b34c226c`. Knowledge, integration, and ingestion `inputs` now inject into their containers during `ast dev`. The `container.Environment` approach was deliberately replaced by `inputs` as the official mechanism.

### 8.3. Inject §8.3 container-mode env vars into agent during `ast dev` (OPEN)

**File**: `apps/astro-cli/internal/compose/builder.go`, `BuildEnvironment()` function (around line 762)

The deployment path (`envresolver.go:resolveKnowledgeConnections`) correctly implements §8.3: custom containers get `KNOWLEDGE_{UPPER(name)}_{HOST/PORT}`. But `BuildEnvironment()` in the compose builder skips custom containers entirely because `GetProvider("").EnvPrefix` is empty.

**Fix** — add an else branch after the provider block at line 761:

```go
// After the existing provider EnvPrefix block:
} else if knowledge.Container != nil {
    // §8.3 — container-mode knowledge, matching envresolver behavior
    prefix := "KNOWLEDGE_" + spec.SanitizeEnvName(name)
    env[prefix+"_HOST"] = &serviceName
    port := fmt.Sprintf("%d", container.Port)
    env[prefix+"_PORT"] = &port
}
```

~6 lines. The same pattern should be applied for container-mode integrations (inject `INTEGRATION_{UPPER(name)}_{HOST/PORT/URL}`), matching `resolveIntegrationConnections()` in envresolver.go.

### 8.4. Add provider health check fallback for knowledge containers (OPEN)

**File**: `apps/astro-cli/internal/compose/builder.go`, knowledge container setup (around line 346)

Models already fall back to the provider's health check when no explicit healthcheck is defined (lines 212-219). Knowledge containers don't. Add the same pattern:

```go
// Before the existing healthcheck block:
healthcheck := container.Healthcheck
if healthcheck == nil && knowledge.IsProviderMode() {
    prov := spec.GetProvider(knowledge.Provider)
    if len(prov.HealthCheck) > 0 {
        healthcheck = &spec.Healthcheck{Test: prov.HealthCheck}
    } else if prov.HealthPath != "" {
        healthcheck = &spec.Healthcheck{Path: prov.HealthPath}
    }
}
if healthcheck != nil {
    // ... existing healthcheck setup code
}
```

~8 lines. This gives the built-in `postgres` provider its `pg_isready` health check automatically.

### 8.5. Use `ServiceConditionHealthy` for services with health checks (OPEN)

**File**: `apps/astro-cli/internal/compose/builder.go`, agent depends_on setup (lines 607-614)

Currently all dependencies use `ServiceConditionStarted`. When a service has a health check, the condition should be `ServiceConditionHealthy`:

```go
for _, service := range project.Services {
    condition := types.ServiceConditionStarted
    if service.HealthCheck != nil {
        condition = types.ServiceConditionHealthy
    }
    dependsOn[service.Name] = types.ServiceDependency{
        Condition: condition,
    }
}
```

~4 lines changed. Combined with §8.4, this means the agent waits for Postgres to accept connections before starting.

### 8.6. Address the `container.Environment` trap (OPEN)

**File**: `packages/astro-spec/parser.go` (validation) or schema generation

The `environment` field on `ContainerConfig` works for models but is silently ignored for knowledge, integration, and ingestion. The team's deliberate direction in `b34c226c` was that `inputs` is the correct mechanism for these sections (surfaced in deploy UI, supports secret storage, prompted via `ast configure`). So the fix should close the gap without reversing that choice:

**Option A (recommended, minimal)**: Add a validation warning when `environment` is set on non-model containers, pointing users to `inputs`. Keeps the team's stated direction; closes the "silently does nothing" footgun.

**Option B (remove)**: Drop `environment` from the schema for non-model containers. Breaking but honest. Better than Option A if the schema can absorb a version bump.

**Option C (reverse course)**: Apply `container.Environment` to all container types, same as models. Matches Docker Compose familiarity but undoes `b34c226c`. Not recommended unless the `inputs`-only policy is being revisited anyway.

Recommendation: Option A, escalating to Option B at the next schema version.

---

## 9. Summary of Recommendations

### Already shipped

| Issue | Status |
|-------|--------|
| `volume` field for custom container persistence | Shipped (`fea6a941`). `container.volume` works with provider fallback. |
| Component inputs injected into sidecar containers | Shipped (`b34c226c`). Knowledge/integration/ingestion `inputs` reach their containers in `ast dev`. |

### Must fix (blocking real usage)

| Issue | Impact | Effort |
|-------|--------|--------|
| §8.3 env vars for custom containers in `ast dev` | Agent can't discover custom knowledge containers; works in deploy but not `ast dev` | ~6 lines in builder.go |
| Provider health check fallback for knowledge | Built-in postgres doesn't get `pg_isready` automatically; models do | ~8 lines in builder.go |
| `depends_on` should use `ServiceConditionHealthy` | Agent starts before DB is ready despite health checks existing | ~4 lines changed in builder.go |
| `container.Environment` trap | Schema accepts it, silently ignored for knowledge/integration/ingestion; works for models | Warning: ~5 lines |

### Should add (common agent patterns)

| Primitive | Why | Complexity |
|-----------|-----|------------|
| Connection string injection (`_URL`) | Every agent constructs this manually; platform has all the pieces | Low |
| Provider + custom image hybrid (documented) | Already works (provider conventions + custom image), just undocumented | Documentation only |
| `ast dev env` command | Eliminates env var guesswork; debugging aid | Low |
| S3/file storage built-in provider | Binary content storage is common; Memory Box had to roll its own MinIO setup | Medium |
| Explicit `uses` for dependency declaration | Makes wiring explicit, enables network scoping in production | Medium |

### Could add (platform differentiation)

| Primitive | Why | Complexity |
|-----------|-----|------------|
| MCP interface support | Becoming standard for AI tool exposure; Memory Box built its own | Medium |
| Schema-managed DB initialization | Most agents need to run SQL on first start; currently app-level | Medium |
| Embedding model built-in provider | Foundational for RAG agents; Memory Box runs Nomic as custom container | Medium |
| Background jobs / cron | Agents need periodic work beyond ingestion triggers; Memory Box built its own job system | Medium |
| Hot-reload dev workflow for frontend agents | Three-process dev setup is painful; agent/ bind mount is too narrow | Medium |
| Platform-level auth primitives | Token management, sessions, secrets; Memory Box built OAuth from scratch | High |

### Naming conventions to document

The env var naming is the most confusing part of the platform. The deployment path (envresolver.go) and the local dev path (builder.go) must converge:

| Scope | Convention | Example | `ast dev` | Deploy |
|-------|-----------|---------|-----------|--------|
| Built-in provider | `{EnvPrefix}_{HOST\|PORT\|URL}` | `POSTGRES_HOST`, `QDRANT_PORT` | Works | Works |
| Custom container (§8.3) | `{SECTION}_{UPPER(name)}_{HOST\|PORT}` | `KNOWLEDGE_DB_HOST` | **Missing** | Works |
| Knowledge inputs | Injected into knowledge container only | `POSTGRES_DB` | Works (since `b34c226c`) | Works |
| Top-level inputs | Injected into all containers | `API_RATE_LIMIT` | Works | Works |

The `ast dev` gap for §8.3 container-mode wiring is the single highest-impact remaining issue. An agent's code should not need environment-specific fallbacks — the same env var names should work in both environments.