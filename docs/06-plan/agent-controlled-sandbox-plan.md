# Agent-Controlled Sandbox — Delivery Plan

Companion to [agent-controlled-sandbox.md](../01-spec/agent-controlled-sandbox.md), which decides the architecture. This one sequences it.

## The sequencing rule

Build every seam right in M1. Start every policy loose. Tighten policy as the audience widens.

Two kinds of decision, and only one is deferrable.

| Structural — right in M1, expensive to change later | Policy — starts loose, tightens on a schedule |
|---|---|
| Control plane API shape, sandbox IDs, addressing | Egress allowlist |
| `envd` as the data plane | Concurrency cap and quotas |
| Deployment-scoped token, derived per-sandbox token | Idle and lifetime ceilings |
| The `running` / `stopped` / `deleted` state machine | What is billed |
| Chunk refcounting, and the runtime tag on stored state | Snapshots, volumes, PTY |
| A sandbox label distinct from the agent label | |
| Dedicated node pool and subnet | |

The two that look deferrable and are not: a refcount retrofitted onto a store that never had one either leaks chunks or deletes live ones; and a sandbox carrying the agent label gets swept by spec-driven reconciliation the day Pods join its list of kinds.

**Isolation is not one of the dials.** gVisor is on from M1. What starts permissive is what a sandbox may *reach*, never what it runs *on*. The thing we actually vary is who is allowed in.

| Audience | Gate |
|---|---|
| Us, internal agents | M1 |
| Design partners | M2 — cost is bounded |
| General availability | M5 — egress is enforced |

## On acceptance criteria

Each milestone below lists criteria that pass or fail without discussion. Anything needing a cluster lands in `astro-server:e2e`; the rest in unit tests. A criterion that cannot be automated is written as a recorded measurement, so at least the number exists and can be compared later.

Numbers given as thresholds are commitments. Numbers described as *recorded* are baselines with no target yet — usually because the milestone that makes them fast has not happened.

## Milestones

### M0 · gVisor soak

Retire the one empirical unknown before anything is built on it.

**Ships** — a gVisor RuntimeClass on a sandbox node pool. No product code.

| # | Acceptance |
|---|---|
| 0.1 | A Node, a Python and a Go project each build to completion on a gVisor node |
| 0.2 | `playwright install` followed by a headless Chromium run completes |
| 0.3 | Twenty concurrent sandboxes for thirty minutes produce no Sentry panic and no unrecovered syscall error in `runsc` logs |
| 0.4 | Every `ENOSYS` or `EPERM` observed is recorded with a workaround or a written accepted limitation |
| 0.5 | Syscall overhead against runc on a build workload is measured and recorded |

### M1 · One sandbox, wide open

**Ships** — pod create and delete from `astro-server`; `envd` reachable on the pod IP; the control plane API; deploy-token auth and the derived per-sandbox token; the sandbox label; a dedicated node pool and subnet; IMDS blocked.

**Still permissive** — all egress allowed, no persistence, no caps, no metering.

| # | Acceptance |
|---|---|
| 1.1 | A deployed agent creates a sandbox, runs `exec`, reads stdout and deletes it — from agent code, not a test harness |
| 1.2 | A token for deployment A returns 404, not 403, for a sandbox owned by deployment B |
| 1.3 | A missing token, a bad signature, and a token for a deleted deployment are each rejected |
| 1.4 | `169.254.169.254` is unreachable from inside a sandbox, and an AWS SDK credential lookup fails there |
| 1.5 | The sandbox pod has no service account token mounted |
| 1.6 | Redeploying the parent agent leaves running sandboxes alive — the sandbox label is not selected by `cleanupOrphanedResources` |
| 1.7 | A sandbox pod without the node selector and toleration stays unschedulable |
| 1.8 | Create-to-first-exec p50 is recorded |

1.6 is a regression test, not a feature test. It is the one that fails silently later.

### M2 · Bounded cost

**Ships** — the state machine; the River reaper; idle and lifetime ceilings; a quota resource counting running sandboxes; `sandbox_compute_usage` billed from transitions; orphan reconciliation in both directions.

**Still permissive** — egress open.

| # | Acceptance |
|---|---|
| 2.1 | An idle sandbox reaches `stopped` within one sweep interval of its ceiling, and the pod is gone |
| 2.2 | A sandbox running past the lifetime ceiling reaches `stopped`; resuming it resets the clock |
| 2.3 | Killing `astro-server` between pod create and row commit leaves no pod after two sweeps |
| 2.4 | Deleting a sandbox pod out of band moves the row off `running`, and `connect` never returns a dead address |
| 2.5 | Deleting a deployment removes every sandbox pod and row it owned |
| 2.6 | Creating past the cap returns 402 with the standard quota body, and the count reflects `running` only |
| 2.7 | Usage over a one-hour run of a known-size sandbox reconciles against wall-clock within 1% |
| 2.8 | Two `astro-server` replicas emit no duplicate usage events for the same interval |
| 2.9 | A sandbox created and killed inside a single sweep interval is billed for its real lifetime, not zero and not a full interval |

2.9 is the whole reason metering integrates over transitions instead of sampling.

### M3 · Persistence

**Ships** — workspace archive to object storage; content-addressed chunks; refcounts; the runtime tag on every chunk set; lazy restore; stop and resume; `sandbox_storage_usage`.

**Deliberately not** — memory checkpoint. The archive is the long-lived format regardless; the checkpoint is an optimisation on top of it.

| # | Acceptance |
|---|---|
| 3.1 | Write a file, stop, resume, read it back unchanged |
| 3.2 | Resume p50 for a 100 MB and a 5 GB workspace differ by under 20% |
| 3.3 | Two sandboxes with an identical dependency tree store under 1.2× the bytes of one |
| 3.4 | Deleting one of them leaves the other resumable |
| 3.5 | A chunk written but not yet referenced by a committed row survives a GC pass |
| 3.6 | Stopping an unchanged workspace stores no new bytes, and the sandbox still resumes |
| 3.7 | A chunk set refuses to restore into a runtime that did not produce it, with a distinct error |
| 3.8 | `sandbox_storage_usage` matches the account partition's real byte count within 1% |

3.2 is the claim the whole storage design rests on. 3.5 is the grace-period race, which is invisible until it eats live data.

### M4 · Product surface

**Ships** — LangChain, Mastra and OpenAI Agents SDK providers; the `ast dev` local control plane; the exec audit stream; `traceparent` forwarded into `envd`.

| # | Acceptance |
|---|---|
| 4.1 | The LangChain provider satisfies `BaseSandbox`, driven through the synthesized `read_file` / `grep` / `ls` toolset rather than `execute` alone |
| 4.2 | The Mastra provider handles spawn, poll output and kill for a background process |
| 4.3 | The same agent source runs against `ast dev` and production with only `SANDBOX_API_URL` differing |
| 4.4 | An exec appears in Langfuse as a child span of the turn that caused it |
| 4.5 | A unique string written to sandbox stdout appears nowhere in any log stream |
| 4.6 | A hostname absent from `egress` fails in `ast dev` with the same error production gives |
| 4.7 | `ast dev --background` leaves sandboxes creatable after the CLI process exits |

4.5 is how "we never log output" stops being an intention. 4.6 is parity of failure — it has to hold even while enforcement is still off in production.

### M5 · Egress, in stages

The one part that has to arrive gradually, because switching it on blind breaks working agents.

| Stage | Ships | Acceptance |
|---|---|---|
| a · Observe | every destination logged, nothing denied | Seven consecutive days of traffic, and a report of distinct hostnames per deployment |
| b · Layer 1 | subnet with no NAT route | With the resolver stopped, no route to any public address exists from a sandbox |
| c · Layer 3 | nftables per-sandbox allow set, CIDRs from the spec | An unlisted address is dropped and the drop appears in the denial stream; a listed CIDR is reachable over a non-HTTP protocol |
| d · Layer 2 | the DNS resolver, hostnames from the spec | An unlisted name returns NXDOMAIN; a listed name resolves, lands in the nft set, and the entry expires on its TTL |
| e · Layer 4 | Envoy, direct `:80`/`:443` dropped | Direct `:443` to an allowed IP is refused; the same request through the proxy succeeds; an allowed IP with a disallowed SNI is refused |

**5.f — every workload catalogued in stage (a) still works after (e).** That is the acceptance criterion for M5 as a whole. The stages are checked against the (a) data before they enforce, so nothing here should be a surprise; if it is, the allowlist model is wrong and not the implementation.

### M6 · Fast resume

**Ships** — gVisor checkpoint/restore, snapshots, fork from a snapshot.

| # | Acceptance |
|---|---|
| 6.1 | A background process running before stop is still running after resume, with its memory intact |
| 6.2 | Restoring a snapshot carrying a dependency tree beats installing that tree; the ratio is recorded |
| 6.3 | N sandboxes forked from one snapshot store only their deltas |
| 6.4 | An open socket is broken on resume as a clean error, not a hang |
| 6.5 | A snapshot outlives the deletion of the sandbox it came from |

6.4 verifies the documented caveat behaves the way the doc promises rather than merely being true.

### M7 · Parity closeout

**Ships** — shared volumes, PTY, filesystem watch.

| # | Acceptance |
|---|---|
| 7.1 | Two sandboxes in one deployment read and write the same volume path |
| 7.2 | A spec referencing another deployment's volume fails to parse |
| 7.3 | A PTY session runs an interactive REPL, handles resize, and delivers Ctrl-C as an interrupt |
| 7.4 | A filesystem watch reports create, modify and delete |
| 7.5 | The capability table in the architecture doc has no unintended gaps left |

## Where we pass Daytona

| Capability | Daytona | Us |
|---|---|---|
| exec and file transfer | yes | M1 |
| Lifecycle and reaping | yes | M2 |
| Persistent workspace | volumes | M3 |
| Framework providers | yes | M4 |
| Egress policy | tier-dependent allowlist | M5, denied by default |
| Snapshots and fork | OCI images in an internal registry | M6 |
| Shared volumes, PTY | yes | M7 |
| Runs inside the customer's own namespace | no | M1 |

The last row is the one they cannot answer, and it is true from the first milestone. Everything above it is catch-up.

## What would make us stop

| Signal | What it means |
|---|---|
| M0 finds a toolchain gVisor cannot run | The isolation choice reopens. `runtimeClassName` makes Kata a field change, but it is a different node pool and a slower start. |
| M5a shows agents reaching hundreds of distinct hosts | The allowlist model does not match real workloads, and the proxy has to become the primary control rather than the fourth layer. |
| 3.2 misses, and resume stays above a few seconds | The pause tier does not pay for itself, `stopped` collapses into `deleted`, and idle cost becomes the product problem again. |
