# Reframe `ingestion` as `schedule`, and make crons configurable from the CLI

## Status — resume here

Last updated 2026-09-06. Working branch: `fix/env-file-and-trigger` in
`astro-cli` (1 commit, **not pushed**, no PR opened yet).

| PR | Branch | State |
|---|---|---|
| 1 — `LoadEnvFile` + trigger flags | `fix/env-file-and-trigger` | **Committed, awaiting Simon's manual test.** `3d0b1c4` |
| 2 — `--schedule` on deploy/redeploy | not started | branch off PR 1 |
| 3 — `configure` collects job inputs + crons, local scheduler | not started | branch off PR 2 |
| 4 — spec `schedule:` alias | not started | `astro-spec`, needs a tag |
| 5 — CLI rename | not started | needs PR 4 released |
| 6 — `command` override | not started | `astro-spec` + CLI |

### What PR 1 actually changed

- `internal/utils/utils.go` — `LoadEnvFile` now requires a regular file, so an
  empty `envFile` no longer resolves to the project directory.
- `cmd/dev.go` — registered `--env` and `-f/--file` on `devTriggerCmd`.
- `internal/utils/utils_test.go` — new; the package had no tests. Two subtests
  fail if the fix is reverted (verified).
- `cmd/docs/ast.md` — documented the new flags.

Verified end-to-end against `slack-radar` by building a binary from `main` in a
throwaway worktree: before, `project trigger` fails 100% of the time with
`read <project dir>: is a directory`; after, it completes.

### Rebuild after a restart

Nothing below survives a reboot:

```bash
cd ~/Documents/GitHub/astro-cli && git checkout fix/env-file-and-trigger
go build -o /tmp/ast-pr1 .                 # the test binary
cd ~/Documents/GitHub/slack-radar && /tmp/ast-pr1 project start -b
```

Then the manual checks for PR 1:

```bash
/tmp/ast-pr1 project trigger                                  # lists both jobs
/tmp/ast-pr1 project trigger discussion_sweep                 # should complete
/tmp/ast-pr1 project trigger discussion_sweep --env .env.staging
```

### Also outstanding, unrelated to this plan

- `astro` monorepo: two commits on `feat/public-demo` (`6a7f2194f`,
  `dcb40f002`), unpushed. Three submodule pointers deliberately left unstaged —
  `modules/astro-infra` would rewind ~10 commits.
- `slack-radar`: clean and pushed.
- `docs/02-cli/cli-command-tree.md` and the Fern pages named in
  `astro-cli/CLAUDE.md` live in the `astro` monorepo, so each CLI PR here needs
  a companion docs change there.
- Simon's Anthropic API key was exposed in an earlier session transcript and
  still wants rotating.

---


## Context

A user asked for "the ability to configure ingestion via CLI — just configure a
cron for an ingestion job", and separately suggested renaming the concept to
`schedule` now that it is no longer really about ingesting data. Both are
correct, and building `slack-radar` surfaced the same gaps from the other side:
a scheduled job could not be run, configured, or scheduled from the CLI at all.

The headline finding from exploring the code is that **the server already
supports CLI-set crons and the CLI simply omits the field**. So the core ask is
a small change with a large payoff, and the surrounding bugs are what make the
feature feel broken today.

## What is actually broken

Verified against `ast/0.17.1`, `astro-cli` @ HEAD, `astro-spec` v0.2.0, and the
`astro-server` source.

1. **The CLI cannot set a cron, and `ast deploy` fails because of it.** The
   server's `TemplateRequest` has `Schedules map[string]string` (ingestion name →
   cron) at `astro-server/internal/deployment/deployment_spec.go:441`, applies it
   at `internal/deployment/template.go:599-606`, and *requires* it — a
   schedule-trigger job with an empty cron is a hard validation error at
   `template.go:663-677`. The CLI's mirror struct at
   `astro-cli/cmd/agent_deploy.go:16-22` does not declare `Schedules`, so
   `ast deploy` on any blueprint with `trigger: {type: schedule}` returns
   `validation.valid == false` with `ingestion.<name>.trigger.schedule: cron
   expression required`, and nothing in the CLI can satisfy it. The interactive
   cron picker exists only in the web client
   (`astro/docs/changelog/ingestion-schedule-ui-2026-03-18.md`).

2. **The cron cannot be read back.** The server returns it in three places and
   the CLI drops all three: the `deployment-template` response root `schedules`
   (CLI's `deployTemplateResponse`, `cmd/agent_deploy.go:52-56`), and
   `WorkloadDetail.Schedule` from `GET /deployments/{id}`
   (`astro-server/handlers/deploy.go:1364`) which the CLI's `workloadDetail`
   struct (`cmd/agent.go:141-146`) does not declare.

3. **`ast project trigger` is broken for every job.** `--env` is registered only
   on `devCmd`/`devStartCmd` (`cmd/dev.go:88-95`) but read by `runDevTrigger`
   (`cmd/dev.go:454`), yielding `""`. `utils.LoadEnvFile` then does
   `filepath.Join(workingDir, "")` → the project directory, whose `os.Stat`
   succeeds, so the "no env file" guard does not fire and `godotenv.Read` fails
   with `is a directory` (`internal/utils/utils.go:29-39`). Passing `--env`
   explicitly fails earlier with `unknown flag`.

4. **`ast project configure` never collects schedule-scoped inputs.**
   `cmd/configure.go` gathers credentials (284-302), custom-provider vars
   (304-320), Slack vars (322-328), top-level `Inputs` (330-343) and
   `Agent.Inputs` (345-352). `spec.Ingestion` is never referenced, so an input
   declared under a job can only come from `.env`. The compose builder *does*
   read it (`internal/compose/builder.go:571-579`), which is why the failure is
   silent rather than loud.

5. **`dev.schedules` is accepted and inert.** Nothing in the CLI schedules
   anything locally. Non-webhook jobs get the `ingestion` compose profile
   (`builder.go:552`), which `projectForUp` strips from `up`
   (`cmd/compose_ops.go:87-98`). The only reader of `spec.Dev.Schedules` in the
   whole CLI is a display line in `ast spec explain` (`cmd/explain.go:271`).
   Combined with (3), there was no way to run a scheduled job locally at all.

6. **A deployed job cannot be triggered manually.** The server has
   `POST /api/v1/deployments/:id/ingestion/:ingestion/trigger`
   (`astro-server/main.go:2299-2306`); the CLI never calls it. `ast project
   trigger` is Docker-Compose-local only.

## Decisions taken

- **Cron is not a spec field.** It stays deploy-time/per-deployment config, set
  by a flag on deploy and by `ast project configure` for local dev. A blueprint
  therefore stays cadence-agnostic, consistent with how watched channels work.
- **The rename is additive.** `schedule:` becomes a synonym for `ingestion:`;
  both parse, old specs keep working, and migration is nudged by a deprecation
  warning. This follows the repo's own precedent.
- **`ComponentKind` stays `"ingestion"`.** It is embedded in generated image
  names (`{agent}-ingestion-{name}`, `astro-spec/pipeline.go:38-43`) and K8s
  workload names (`Suffix()`, `pipeline.go:27-32`). Changing it orphans pushed
  images and in-cluster resources for zero user-visible gain.

## Plan

### Phase 1 — Make crons work from the CLI (highest value, smallest diff)

**1a. Send and read back the cron.** In `astro-cli`:

- Add `Schedules map[string]string \`json:"schedules,omitempty"\`` to
  `deployTemplateRequest` (`cmd/agent_deploy.go:16-22`) and `Schedules
  map[string]string \`json:"schedules"\`` to `deployTemplateResponse`
  (`cmd/agent_deploy.go:52-56`).
- Add `--schedule name=expr` (repeatable) to `agent_deploy.go` and
  `agent_redeploy.go`, parsed by a new helper alongside the existing
  `parseDeployVars` (`cmd/agent_deploy.go:107-199`) — same shape, so follow that
  function rather than inventing a new convention.
- Add `schedule` to the CLI's `workloadDetail` struct (`cmd/agent.go:141-146`)
  and surface it in `ast agent get`, so the current cadence is visible.
- Validate the expression client-side before the round trip, using the **same
  5-field parser the server uses** (`cron.Minute|Hour|Dom|Month|Dow`,
  `astro-server/internal/deployment/template.go:751-755`) so anything the CLI
  accepts the server also accepts. Add `robfig/cron/v3`; neither `astro-cli` nor
  `astro-spec` has a cron parser today.

Note the merge order on the server is favourable and needs no change: prefill of
the stored cron (`handlers/deploy.go:4196-4204`) runs *before* `req.Schedules`
is applied, so omitting the flag preserves the existing cadence and passing it
overrides — which is exactly the desired semantics, and it removes the current
delete-and-redeploy requirement for a cadence change.

**1b. Fix `LoadEnvFile` and the missing flag.** In `internal/utils/utils.go:29`,
require a regular file:

```go
info, err := os.Stat(path)
if err != nil || !info.Mode().IsRegular() {
    return nil, nil          // no env file is not an error
}
```

This fixes every caller, present and future. Separately register `--env` (and
`--file`) on `devTriggerCmd` so the flag can actually be passed.

**1c. Collect schedule-scoped inputs in `configure`.** Add a fourth gather pass
in `cmd/configure.go` after the `Agent.Inputs` loop (line 345-352), iterating
`astroSpec.Ingestion` and its `Inputs`, labelled with the owning job so a name
appearing in two jobs is distinguishable. `varEntry` (line 64-69) already
carries everything needed. `astro-spec`'s env resolver already scopes these
correctly (`envresolver.go:177-183`) — this is purely a CLI-side gap.

**1d. Collect crons in `configure`, and honour them in `ast project start`.**
`ProjectConfig` (`internal/config/storage.go:13-16`) holds only
`Vars map[string]string`, with no room for non-`KEY=VALUE` state. Add a sibling
field rather than smuggling crons in as pseudo-env vars:

```go
type ProjectConfig struct {
    Name      string            `json:"name"`
    Vars      map[string]string `json:"vars"`
    Schedules map[string]string `json:"schedules,omitempty"`   // job name → cron
}
```

`configure` prompts for a cron per schedule-trigger job (pre-filled from
`dev.schedules` when present, so the spec value becomes the default rather than
dead config).

`ast project start` then runs a scheduler that fires `RunOneOffContainer` — the
same call `runDevTrigger` already uses (`cmd/dev.go:532-536`) — on each cadence.
**Copy the chat-UI worker pattern rather than using an in-process ticker**, so
schedules also fire under `ast project start -b`, which exits immediately
(`cmd/dev.go:329-332`). `startChatUI` (`cmd/chatui.go:102-175`) already does
exactly this and is described in `cmd/dev.go:320-322` as "a detached worker so
it survives background mode". Mirror its five moving parts:

- a hidden subcommand (`chatui-serve`, `cmd/chatui.go:39`) — add a
  `schedule-serve` equivalent
- re-exec of `os.Executable()` (`chatui.go:113`) with args
- a log file under the project's `.ast` dir (`chatui.go:119-125`)
- a pid file (`chatUIPidFile = ".chatui.pid"`, `chatui.go:31,152`)
- `stopChatUI` (`chatui.go:179`), which verifies the pid is still the right
  process by inspecting its cmdline (`chatui.go:198-209`) — wire the equivalent
  into `ast project stop`

This removes the awkward "crons are accepted but inert in background mode"
caveat entirely, and reuses a lifecycle that already handles the leaked-worker
and stale-pid cases.

**1e. Add `ast agent trigger <job>`** calling the existing server endpoint
`POST /api/v1/deployments/:id/ingestion/:ingestion/trigger`
(`astro-server/main.go:2299-2306`), reusing `resolveAgentTarget`
(`cmd/agent_target.go:25-74`). This is what makes a deployed schedule testable
without waiting for its cadence, and it is the deployed counterpart to
`ast project trigger`.

### Phase 2 — The additive rename

Do this *after* Phase 1 so the bug fixes are not blocked behind a
cross-repo release.

**In `astro-spec`** (needs a tagged release; `astro-cli` pins
`astro-spec v0.2.0` with no `replace` directive):

- Add `Schedule map[string]Ingestion` to `AstroSpec` (`spec.go:22`) alongside
  the existing `Ingestion` field, and a resolver method
  `ResolvedSchedules()` that prefers `Schedule` and falls back to `Ingestion` —
  exactly the `Models`/`Model` + `ResolvedModels()` pattern
  (`spec.go:118-134`). Every internal reader moves to the resolver.
- Reject both keys being set at once in `ParseSpec`, mirroring the
  `models`/`model` mutual-exclusion error (`parser.go:164-166`).
- Add a `DeprecationWarnings` entry (`parser.go:335-341`) for `ingestion:`.
  That channel exists and currently has one entry, so this is its second use
  rather than new machinery.
- Rename the type `Ingestion` → `ScheduledJob` (and `IngestionTrigger` →
  `JobTrigger`) with type aliases left behind, since the CLI and server both
  reference the names.
- Regenerate `astropods.schema.json` — it is a committed, `go:embed`-ed artifact
  (`schema.go:7-13`) and the root object is `additionalProperties: false`, so an
  unregenerated schema makes `schedule:` an editor-level error. Note **CI does
  not verify schema freshness**; worth adding a `go generate` + `git diff` step
  in the same change.
- The spec is also persisted as JSON elsewhere (per the `DevInterfaces`
  `UnmarshalJSON` comment, `spec.go:385-400`), so the JSON tags matter as much
  as the YAML ones.

**In `astro-cli`**: bump the spec dependency, move reads to
`ResolvedSchedules()`, rename user-facing vocabulary (`ast add schedule` with
`ingestion` kept as a hidden alias, `ast project trigger` help text), and leave
load-bearing internal strings alone — the `ingestion-` compose service prefix
(`builder.go:533`, `cmd/dev.go:533,590`), the `ingestion` compose profile
(`builder.go:552`), and the `ingestion/<type>/` scaffold layout
(`internal/scaffold/scaffold.go:229-296`). Renaming those buys nothing and
breaks existing projects on disk.

### Phase 3 — Scheduled runs without a second image

**Recommendation: allow a `command` override on a scheduled job, not prompt
injection into the agent.** `lorren-archivist` is the requesting case and the
evidence there is one-sided.

What that repo actually looks like:

- It already has two ingestion containers, `schedule` and `manual`
  (`lorren-archivist/astropods.yml:83-97`), and
  `ingestion/schedule/index.ts` and `ingestion/manual/index.ts` are
  **byte-for-byte identical except one line** —
  `readRunOptionsFromEnv('schedule')` vs `('manual')`. Two Dockerfiles exist to
  differentiate one string literal.
- The work is already a clean one-shot function,
  `runLorrenArchivistAgent(options)`
  (`lorren-archivist/runtime/orchestrator.ts:167`), whose options come purely
  from env (`runtime/options.ts:77-92`). No chat turn is involved anywhere.
- **When the team had full control they chose the command-override model.**
  Their own CDK stack runs one image with
  `command: ['bun', 'ingestion/schedule/index.ts']`
  (`infra/aws/lib/agent-stack.ts:165`) and binds a cron with
  `scheduler.CfnSchedule` (`agent-stack.ts:226-231`). They did not build two
  images in AWS. They went outside the platform precisely because
  `trigger: {type: schedule}` carries no cron — which is the strongest possible
  argument for Phase 1.

Why prompt injection is the wrong model, at least for this case:

- The root `agent` image **cannot do the work**: its Dockerfile copies only
  `agent/` (`lorren-archivist/Dockerfile:5`), so the workflow code is not in it.
- There is no chat surface to inject into. `agent/index.ts` is a 43-line health
  stub serving `/health` and `/`, whose own response says the agent is
  "batch-only" (`agent/index.ts:18-33`), confirmed by `README.md:202`.
- **Exit codes are load-bearing.** A failed run must exit non-zero
  (`runtime/orchestrator.ts:194-196`, `ingestion/schedule/index.ts:43-51`), and
  a conversation turn has no equivalent — a scheduled agent prompt would report
  success for a failed run.
- Runs are stateful across ticks: the DynamoDB activity ledger means run N+1's
  input scope depends on run N's outcome (`workflow/delivery-ledger.ts:39-56`,
  `README.md:120`), and they deliberately set `maximumRetryAttempts: 0` because
  duplicate delivery is the feared failure. Exactly-once semantics matter more
  than invocation ergonomics.

So the change is small and mostly in the spec:

- Add `command []string` to `ContainerConfig` (`astro-spec/spec.go:248-257`,
  which today has image/build/gpu/port/volume/environment/healthcheck but no
  command). Precedent exists for a command field in `Dev.Command`
  (`spec.go:268`).
- Allow a scheduled job to omit `container.build`/`image` and inherit the
  agent's image, supplying only `command`. That collapses
  `lorren-archivist`'s two Dockerfiles into two lines of config and makes
  `slack-radar`'s `Dockerfile.digest` (which exists solely to change `CMD`)
  unnecessary.
- Keep parameterisation as declared inputs, which is already how both repos do
  it and how `envresolver.go:177-183` scopes them. **No per-tick payload** —
  neither requesting case wants one.

One caveat worth recording: `lorren-archivist/astropods.yml:99-102` declares
`dev.interfaces.messaging.adapters: [web]` for an agent with no chat endpoint.
Any platform feature that keys off "does this agent declare messaging?" will get
a false positive there, which is a second reason not to build scheduled runs on
the messaging path.

Prompt-scheduled agent runs may still be worth having later for genuinely
conversational agents — `slack-radar`'s `lead_digest` is arguably one. It is a
separate feature and should not gate this one.

### Phase 3 verification note

`command` is a spec addition, so Phase 3 rides the same `astro-spec` release as
Phase 2 rather than needing its own.

## Verification

Each phase is testable end-to-end against `slack-radar`, which has two
schedule-trigger jobs (`discussion_sweep`, `lead_digest`) and is already
deployed.

1. **Cron on deploy.** `ast deploy` a schedule-trigger blueprint with no
   `--schedule` and confirm the validation error is the current behaviour; then
   with `--schedule discussion_sweep="*/15 * * * *"` and confirm it deploys.
   `ast agent get` should show the cadence. Re-run `ast agent redeploy` with no
   flag and confirm the cron survives; with a new value and confirm it changes —
   that is the delete-and-redeploy requirement going away.
2. **Bad cron rejected locally.** `--schedule x="not a cron"` should fail before
   any network call, with the same verdict the server would give.
3. **`ast project trigger`.** Run it in `slack-radar` (it currently fails 100%
   of the time) and confirm the job runs; then with an explicit
   `--env other.env`.
4. **Schedule-scoped inputs.** `ast project configure` in `slack-radar` should
   now offer `RADAR_SLACK_BOT_TOKEN` (a `discussion_sweep` input). Confirm it
   reaches the container with `docker exec … printenv`.
5. **Local scheduler.** Configure a `* * * * *` cadence, `ast project start` in
   the foreground, and confirm the job container runs each minute without
   manual triggering. Confirm `-b` prints the warning.
6. **Deployed trigger.** `ast agent trigger discussion_sweep` and confirm a run
   appears in `ast agent logs --workload discussion-sweep`.
7. **Command override.** Point `lorren-archivist`'s two jobs at the agent image
   with `command: [bun, ingestion/schedule/index.ts]` and confirm both run
   identically to their current dedicated containers, including a non-zero exit
   on failure. Then delete `slack-radar`'s `scheduler/Dockerfile.digest` and run
   `lead_digest` via `command` instead.
8. **Rename back-compat.** Existing `slack-radar` `astropods.yml` (which uses
   `ingestion:`) must parse unchanged and emit one deprecation warning; a copy
   switched to `schedule:` must behave identically; a spec with both keys must
   fail with a clear error. `go generate ./... && git diff --exit-code` must be
   clean in `astro-spec`.

## Release coordination

`astro-cli` pins `astro-spec v0.2.0` with no `replace` directive or `go.work`,
so Phase 2 needs: tag `astro-spec`, bump `astro-cli`'s `go.mod`, release the
CLI. Phase 1 touches only `astro-cli` and needs no spec change, which is the
main reason to ship it first. Note `astro-spec` HEAD is already ahead of
`v0.2.0`.
