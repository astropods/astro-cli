# Agent-controlled sandbox — architecture research

## Summary

Agents have no way to run untrusted code, install packages, or keep a workspace outside their own container. This adds `docs/05-architecture/agent-controlled-sandbox.md`, covering the landscape and the architecture we intend to build.

This is a different question from `container-vs-microvm.md`. That doc asks how to harden the box the agent itself runs in; this one asks what second box we hand the agent to drive as a tool.

## Design

**We build it rather than resell a hosted sandbox.** Reselling puts the one place customer code and data actually execute outside our infrastructure. The differentiator is not that the sandbox reaches private services — by default it reaches nothing — but that it runs inside the customer's boundary at all.

**The framework contract is tiny.** Mastra, LangChain, the OpenAI Agents SDK and Anthropic Managed Agents all converged on `exec(command)` plus file transfer; every filesystem tool the agent sees is synthesised on top by the framework. We adopt E2B's `envd` protos (Apache-2.0, ConnectRPC over HTTP/2) rather than designing our own, so existing framework providers work against us with a base-URL change.

**Placement is the deployment's own namespace**, on gVisor, with the agent talking directly to the sandbox pod IP. The control plane creates and reaps but stays out of the data path.

**Egress is four layers**, each holding if the one above fails: a dedicated subnet and security group via VPC CNI custom networking; a DNS resolver we control that only answers allowlisted names and programs the results into an nftables set; the nftables set itself, which matches on address so any protocol works; and Envoy, made mandatory for `:80`/`:443` so TLS gets an SNI check. IPv4 only, because `ENIConfig` cannot do IPv6 and layer 1 depends on it.

**The spec carries only what the platform cannot infer** — `egress`, `grants`, and `volumes`, all name lists. Image, resources, timeouts and workspace path are platform defaults and tier policy.

**Auth reuses the deploy token.** `SANDBOX_API_TOKEN` is the existing HS256 deployment JWT, so no second credential scheme. The per-sandbox token cannot be a JWT because envd compares a fixed string, so it is derived as an HMAC over sandbox ID and instance number — nothing stored, and a resume necessarily reissues. The sandbox pod deliberately does not get the restricted Pod Security Standard the rest of the platform applies: a sandbox that cannot install packages is not a sandbox, and gVisor is what makes root inside it acceptable.

**`ast dev` gets a local control plane, not a compose service per sandbox.** Sandboxes are runtime-created and appear in no compose project, so the CLI runs the same handlers against the Docker API — shared as a package between `astro-server` and the local runner. The data plane is byte-identical (same image, same daemon, same protocol), so framework provider code does not change between dev and production. Isolation, egress and checkpointing all diverge locally; the rule is parity of failure rather than parity of mechanism — anything production denies must fail locally with the same error.

**Metering reuses CU-hours but not the sampling.** Sandboxes emit the same unit the deployment heartbeat does, integrated over row timestamps rather than sampled — a sandbox often starts and dies between two five-minute ticks, so sampling either misses it or rounds it up. Storage is deduplicated within an account and never across, so one account's bill cannot move when another deletes. The concurrency cap becomes a `quota.DBChecker` resource checked on create and on resume.

**Observability turns on the exec audit, not pod logs.** Command output returns over the protocol, so daemon stdout carries nothing about the agent's work; we log command metadata and never output. Traces require `traceparent` forwarded into envd, otherwise a slow exec has no relationship to the turn that caused it. Egress denials get their own stream — layers 2 and 3 drop silently, so a correctly blocked sandbox is otherwise indistinguishable from a broken network.

**Cleanup reuses the River periodic-job pattern.** The pod and the DB row can each outlive the other, so the sweep reconciles both directions rather than walking rows. Two constraints fall out: deduplicated chunks need refcounting with a grace period, since a chunk is written before the row referencing it commits; and sandboxes must stay out of `cleanupOrphanedResources`, which deletes anything with the agent label that the spec does not imply — a sandbox is runtime state and appears in no spec.

**Lifecycle is agent-driven, platform-reaped.** The agent creates sandboxes and holds the ID; we enforce idle and lifetime ceilings. No session concept is needed. Workspace lives on node-local disk and is archived to S3 on stop, because network-attached storage — EFS, S3 Files — loses on the small-file IOPS that dev workloads are made of.

**A second isolation runtime stays open, and costs one thing today.** Kata is the affordable second tier — raw Firecracker means a second control plane beside EKS. The API, `envd`, the spec and three of the four egress layers would not notice the switch; what it costs is a permanent bare-metal node pool and stop/resume implemented twice, since gVisor checkpoints at the Sentry and Firecracker snapshots the VM. The only part worth paying for now is tagging stored state with the runtime that produced it, because a cross-runtime restore without that tag corrupts a workspace instead of being refused.

**Delivery is sequenced by what cannot be retrofitted, not by feature size.** `docs/06-plan/agent-controlled-sandbox-plan.md` splits the design into structural decisions that must land in the first milestone — API shape, token model, state machine, chunk refcounting, the sandbox label, the dedicated subnet — and policy that starts permissive and tightens: egress, quotas, ceilings. Isolation is explicitly not one of the dials; gVisor is on from M1 and what varies instead is the audience, gated on cost being bounded (M2) and egress being enforced (M5). Egress itself arrives in five stages beginning with observe-only, so each enforcement layer is checked against real traffic before it denies anything.

## Migration

None. This is a research and design document; no code or configuration changes.
