# Project Overview

Astro is a platform for deploying and running AI agents of any kind. It provides agent-native infrastructure including models, knowledge bases, tool integrations, and observability. Agents can be packaged as containers and deployed with declarative configuration.

The project is a monorepo that contains packages for agent deployment infrastructure and optional utilities for building agents.

IMPORTANT: When planning and creating spec, please ensure it's concise and complete while trading off verbosity and grammar. Avoid putting code examples unless necessary.

# Documentation

[`docs/README.md`](docs/README.md) indexes every doc folder and maps feature
areas to their canonical doc. Check it before answering "how does X work" or
adding a new doc — an existing one under `docs/01-spec` or `docs/03-architecture`
may already cover the area and just need updating. When a change makes a
`03-architecture` doc wrong, fix it as part of the same change. A `01-spec`
doc is a design record, not a live description; if it disagrees with what
you built, banner it and point at the doc that now describes the as-built
system, rather than rewriting it to match. See `docs/README.md`'s "The
rule" for the full breakdown across every doc folder.

**When delegating a coding task to a subagent, say so in its prompt.** A
subagent doesn't inherit this file, the `docs-map` skill, or the docs-map
hook (`.claude/hooks/docs-map-check.mjs`) — it only sees what its own
prompt gives it. If the task touches a path
listed in `docs/README.md`'s area map, name the relevant doc(s) in the
delegating prompt and ask the subagent to check and fix them, the same way
you would if doing the work directly. If the doc is being cited to override
code that looks different, ask the subagent to verify that specific claim
against current code first, not just cite the doc — a stale or wrong claim
shouldn't propagate into new code with more confidence than the code it's
replacing.

# Apps

The Apps/Infrastructure/Packages tables below are hand-maintained, unlike the
Moon target list — they carry real prose (purpose, submodule setup) a command
can't produce, so they stay as tables rather than a "run this command"
pointer. That also means they drift like any other doc when a path moves;
`astro-identity-gen`'s path was wrong here until a 2026-08-26 sweep caught it.
If something in these tables looks off, verify against the actual tree
before trusting it.

| App | Path | Purpose |
|-----|------|---------|
| astro-cli | `modules/astro-cli` | CLI for building, pushing, and deploying agents; handles local dev mode, container builds, registry push, and spec registration. Private git submodule (`astropods/astro-cli`); run `git submodule update --init modules/astro-cli` to work on it |
| astro-client | `apps/astro-client` | React web frontend for managing agents, deployments, observability, and team settings |
| astro-queen | `apps/astro-queen` | Admin console: a Cobra CLI that serves an embedded React SPA (`//go:embed web/dist`) backed by the AdminService gRPC API; provides access to cluster status, deployments, jobs, and observability |
| astro-registry | `apps/astro-registry` | Docker Registry V2 API proxy with auth; routes push/pull operations to backend registry (ECR) with membership checking |
| astro-server | `apps/astro-server` | Go backend API server handling agent registry, K8s deployments, auth (WorkOS), admin gRPC, and observability |
| astro-otel | `apps/astro-otel` | OTLP ingest service for local AI coding tools (e.g. Claude Code); authenticates account-scoped ingest keys against the DB and forwards traces→Langfuse and metrics→VictoriaMetrics |
| tests | `apps/tests` | Playwright smoke/e2e suites; run via `moon run tests:smoke` (see repo README Smoke Tests) |

# Infrastructure

| Module | Path | Purpose |
|--------|------|---------|
| astro-infra | `modules/astro-infra` | Terraform + Kubernetes infra for the managed cluster: VPC, tenant router, egress proxy, cost tracking, BYOC onboarding. Has its own `docs/architecture/`, `docs/decisions/`, `docs/runbooks/`, and `docs/plans/` — not part of this repo's `docs/` tree or area map. Private git submodule; run `git submodule update --init modules/astro-infra` to work on it |

# Packages

| Package | Path | Purpose |
|---------|------|---------|
| astro-collector | `packages/astro-collector` | OpenTelemetry Collector distribution for collecting traces and metrics from deployed agents |
| astro-proto | `packages/astro-proto` | Protobuf definitions and generated code for gRPC services (AdminService API) |
| astro-spec | `packages/astro-spec` | YAML spec parser and types for `astropods.yml`; shared by CLI and server to parse/validate agent configuration. Public git submodule (`astropods/astro-spec`), consumed as Go module `github.com/astropods/astro-spec`; run `git submodule update --init packages/astro-spec` (keyless) to work on it |
| astro-brand-icons | `packages/astro-brand-icons` | Brand icon set with a build pipeline (`sources/` + `icons.json`) producing icon components |
| astro-identity-gen | `apps/astro-server/internal/identitygen` | Procedural, deterministic identity/avatar image generation. Not a standalone package (moved from `packages/astro-identity-gen`, a Go internal package now) |
| astro-theme | `packages/astro-theme` | Shared UI theme (design tokens and CSS) |
| astro-trading-card | `packages/astro-trading-card` | Agent trading-card rendering (SVG/PNG export, holo effects) |
| blueprint-jellybean | `packages/blueprint-jellybean` | Blueprint card rendering assets |

# Tooling

This is a bun monorepo. Always use `bun x <command>` instead of `npx`.

## Moon

Use Moon as the default task runner from repo root: `moon run <project>:<task>`.
Run `moon query tasks` to see the current target list — don't rely on a
memorized or cached one, tasks are added/renamed often enough that a written
list here would already be wrong (it was, the last time this was checked: it
was missing `astro-server:test-billing` and `bifrost-otel:test`, both real
targets).

# Kubernetes API Usage

**K8s queries are for live status only, not for fetching data we already wrote.** When the server needs deployment/workload state (replica counts, pod phase, container readiness, restart counts, conditions), querying K8s is the only authoritative source — use it. But when the server already owns the data (env vars, spec, intent, URLs), read from the database instead.

Why: K8s API reads are not free. A single deployment-detail render that walks per-workload Secrets and ConfigMaps multiplies into dozens of GETs per poll per viewer; at scale that becomes a sustained load problem on the cluster, and during rolling updates the K8s view also lags the deployed spec until pods cycle. The DB is the apply-time intent — it doesn't lag, doesn't rate-limit, and we wrote it on purpose. Defaulting to K8s "because it's there" is exactly the trap to avoid.

Rule of thumb when adding a server-side read: if the question is *"what is the cluster doing right now?"* → K8s. If the question is *"what did we deploy?"* → DB. Don't conflate the two; the runtime endpoint and the record endpoint exist precisely to keep them separate (`apps/astro-server/handlers/deploy.go`).

# Data Fetching (astro-client)

All server data integration uses TanStack Query. See [docs/04-guides/tanstack-query.md](docs/04-guides/tanstack-query.md) for architecture, conventions, and best practices. Key rules:
- Never call `api.*` directly in components for reads — use query hooks from `src/api/queries/`.
- All query keys must come from the factories in `src/api/queries/keys.ts`.
- Mutations invalidate affected queries in `onSuccess`.

# Log messages (Go)

Every log message is `component: lowercase phrase`, with everything variable in
structured fields. The component is the subsystem the reader would grep for,
usually the file's own concern (`deploy:`, `river:`, `insights rollup:`).

```go
log.Info("river: registered worker", "worker", "DeployWorker", "period", "5m")
log.Warn("deploy: resolve cluster ingress config failed", "cluster_id", id, "error", err)
```

- Lowercase the phrase. Identifiers keep their own case: `K8s`, `WorkOS`,
  `METRONOME_API_KEY`, `StatefulSets`.
- Name the failure at the end, not the front: `get deployment failed`, not
  `Failed to get deployment`. Failures then sort next to their subject.
- One colon. The component prefix owns it, so separate a reason with a comma:
  `deploy: rejected, cluster unhealthy`.
- Never interpolate a value into the message. A message is a constant so it can
  be grepped and grouped; `"account_id", id` is a field.
- Field keys are snake_case.
- Don't repeat the component in the phrase: `billing collect: completed`, not
  `billing collect: billing collect`.

# Writing style

Applies to code comments, changelog entries, release notes, and specs. For
public product documentation, `docs-public/AGENTS.md` wins.

Follow the [Google developer documentation style guide](https://developers.google.com/style),
plus these house rules:

- **Active voice, present tense.** "The worker retries the job", not "the job
  will be retried".
- **One idea per sentence.** 20 words or fewer for a procedural sentence, 25 for
  a descriptive one. Split a long sentence instead of joining clauses.
- **One approved term per concept.** Reuse the same word every time. Do not vary
  wording for variety; in technical text a synonym reads as a second thing.
- **Second person and imperative for instructions.** "Run the migration", not
  "the migration should be run". Sentence-case headings.
- **Comments are rare and short.** Add one only when the code would genuinely
  confuse a reader without it, never to restate what the next line does.
  When one is warranted, write the least that resolves the confusion — a
  phrase or one sentence, not a paragraph. Most functions need none; several
  comments in one body usually means the code needs better names.
- **A comment describes the code as it is, not its history.** No "this was
  broken before", "as discussed", "we discovered", "once support enables
  this", no dated status, no bug-hunt history, no explaining why the change
  was made. That belongs in the changelog, not the comment.
- **No em dashes.** Use a comma, a colon, a pair of parentheses, or a second
  sentence. An em dash usually joins two ideas that read better apart, so
  removing it tends to satisfy the one-idea rule at the same time.
- **A checkable claim says how to check it.** When a doc states something a
  reader could verify — a count, a named function or constant, a quoted
  line, an env var — say what would confirm it: a file, a test, a command.
  "6 indexes" is weaker than "6 indexes (see internal/auditlog/schema.sql)".
  It costs a few words now and saves someone re-deriving the claim from
  scratch during the next audit.

Changelogs carry more prose than a comment, because they explain a design and
the reason for it. The rules above still apply: a changelog describes the
system after the change, not the process that produced it.

Existing text is not being rewritten to match. These rules apply to new and
changed lines, so a file can hold both styles while it turns over.

# Development Workflow

Follow this end to end for any change that touches code (skip the
verification/testing steps for a pure doc or config-only change):

1. **Find the relevant docs first.** Check [`docs/README.md`](docs/README.md)'s
   area map and the relevant app's own `CLAUDE.md` (e.g. `apps/astro-client/CLAUDE.md`),
   if one exists, before making the change, not just after — know what
   "correct" looks like before touching code, not after a neighboring file
   has already shaped the approach.
2. **Prefer the existing convention over a new one.** Reuse or extract a
   component instead of duplicating one; don't hand-roll a value the theme
   already has a semantic token for; don't invent a new error-handling,
   route-wiring, or data-fetching shape when one's already established
   nearby (the `Store` + sentinel-error pattern and `internal/openapi` route
   wiring in Go, TanStack Query hooks and semantic Tailwind tokens on the
   frontend — see [`docs/04-guides/go-store-pattern.md`](docs/04-guides/go-store-pattern.md),
   [`docs/04-guides/openapi-route-wiring.md`](docs/04-guides/openapi-route-wiring.md),
   [`docs/04-guides/tanstack-query.md`](docs/04-guides/tanstack-query.md),
   and `apps/astro-client/CLAUDE.md`). **A documented convention wins over
   nearby code that doesn't follow it.** Code that predates or ignores a
   convention is inherited drift, not precedent — match the doc, and either
   fix the deviation in what you're touching or log it, rather than copying
   it forward into new code. Before leaning on a doc's specific load-bearing
   claim to justify overriding what's in front of you, check that one claim
   against current code first. The convention wins; a stale or invented
   detail inside the doc describing it doesn't.
3. **When you deviate from a convention or a documented design, resolve
   it — don't leave it silent.** Either bring the code back in line with the
   convention/doc, or update the doc to reflect the new reality, whichever is
   the better outcome for the codebase. Never ship a change that quietly
   disagrees with what's documented or with the pattern used everywhere else
   nearby.
4. **Verify the change for real, not just that it compiles or typechecks.**
   Start whatever it takes to observe the change working and confirm the
   behavior actually changed as intended. Iterate on failures instead of
   declaring the task done. If something can't be verified from where you're
   running (missing infra, a live account/cluster you don't have access to,
   etc.), say so plainly rather than assuming it works.
5. **Once the change is verified, bring tests up to it.** Add or update tests
   for the behavior you changed, aiming for real coverage of it, not a padded
   percentage. Run the suite and fix what it finds before calling the work
   done. For Go, follow the existing shape for the kind of test you're
   adding (mocked-DB, real-Postgres integration, or pure-unit) — see
   [`docs/04-guides/go-testing-conventions.md`](docs/04-guides/go-testing-conventions.md).
6. **When the scope of work is complete, write the changelog** (use the
   `write-changelog` skill). This is also the point to do the doc-vs-code
   check from step 3 one more time, now that the whole change is in view.

Comments stay governed by the writing-style rules above — minimal,
explaining what the code does now and why, never narrating the change that
produced it. If something about *why this change happened* is worth
recording, it belongs in the changelog, not a comment.

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
3. For each area those changelogs touch, check whether it has a canonical doc in [`docs/README.md`](docs/README.md)'s area map, and whether the change makes that doc stale. Fix it in the same pass if so — this is the one point in the normal workflow where a whole commit range's worth of changes gets looked at together, so it's the cheapest place to catch drift that a single-PR pass would miss.
4. Write the release note with two sections:

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

5. Commit the release note file, then tag the commit:
```
git tag release/YYYY-MM-DD.N
```
The tag should match the filename exactly (e.g. `release/2026-04-10.2`).

# Product Documentation

Public product documentation lives in `docs-public/fern` (Fern). When writing or editing it, follow [docs-public/AGENTS.md](docs-public/AGENTS.md) — the source of truth for voice, structure, naming (always "Astro AI", never bare "Astro"), and what must not appear (internal rationale, component names, roadmap). See `docs-public/README.md` for local preview.