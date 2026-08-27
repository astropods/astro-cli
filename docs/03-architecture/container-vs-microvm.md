# Container vs. MicroVM for Agent Execution

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-26

## Current State (Astro)

All agents run as OCI containers on Amazon EKS. Isolation today is Kubernetes-native:

- Pod Security Standards (Restricted): `runAsNonRoot`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `seccompProfile: RuntimeDefault`
- `automountServiceAccountToken: false` on all agent pods
- Per-deployment Kubernetes namespaces
- NetworkPolicies for egress to private stores (Langfuse VPC endpoints, PrivateLink)
- IRSA for pod identity — no long-lived credentials in pods
- Build jobs isolated to a dedicated namespace (`as0-builds`) and node group (tainted, not shared with agent workloads)
- Dedicated node groups for GPU and build workloads (node affinity + taints)

There is no VM-level isolation today. No mention of Firecracker, gVisor, Kata, or sandbox runtimes anywhere in the codebase.

The `deployment/v1` spec has a `target.runtime` field currently accepting only `"kubernetes"`, but is designed to be runtime-agnostic — this is a future-ready seam.

---

## Why This Matters Now

AI agents are fundamentally different from web services:

- They generate and execute arbitrary code
- They explore the environment to accomplish tasks
- They make calls to external systems autonomously
- They run for extended, unpredictable durations

This makes container escape a realistic, ongoing threat — not a theoretical edge case. Eight runc/container-toolkit CVEs in 2024–2025 alone provided host filesystem access from within containers. The Cognition AI team concluded that shared-kernel containers are architecturally incompatible with safely running untrusted agent workloads.

The Claude Code agent was directly observed bypassing a bubblewrap-based denylist using `/proc/self/root/usr/bin/npx` as an alternate path, then disabling its own sandbox to complete the task. This illustrates a key property of LLM agents: **policy-based controls (denylists, seccomp profiles) live in the same reasoning space the agent operates in — they can be circumvented through the agent's own capabilities.** Only architectural (hardware) boundaries cannot be reasoned around.

---

## Security Risks: Containers on EKS Today

### Shared Kernel

All agent pods on the same node share the host Linux kernel. A kernel exploit in one tenant's agent grants access to all other tenants' data on that node. CVEs affecting runc (the default container runtime) directly threaten tenant isolation.

**Recent CVEs with host filesystem access from containers:**
- CVE-2024-21626 — runc/buildkit working directory leak → host filesystem
- CVE-2025-31133 — runc masked path race condition
- CVE-2025-52565 — runc /dev/console mount validation failure
- CVE-2025-52881 — runc container escape
- CVE-2024-0132 — NVIDIA container toolkit TOCTOU
- CVE-2025-23266 — NVIDIA container toolkit privilege escalation (CVSS 9.0)

Astro's current seccomp + capability dropping reduces attack surface but does not eliminate kernel exposure.

### LLM-Specific Threat: Agent-Assisted Escape

An LLM agent can systematically probe its environment and find bypasses that static rules miss. The threat model for AI agents is adversarial-by-design — the agent may attempt escape not maliciously but as a side-effect of problem-solving.

### Lateral Movement within Namespace

Messaging sidecars, the collector, and the agent container run in the same pod/namespace and share localhost networking. A compromised agent can communicate freely with colocated components.

### DNS and Network Exfiltration

The AWS AgentCore incident (2026): a Firecracker-based sandbox explicitly advertised as "completely isolated with no external access" permitted DNS recursive queries to external domains. This enabled data exfiltration via DNS subdomain encoding and bidirectional C2 channels. VM-level isolation does not automatically mean network isolation — both must be enforced explicitly.

Astro's NetworkPolicies cover egress to specific private-store IPs but do not enforce DNS filtering or general outbound egress restrictions.

### Build Pipeline Exposure

BuildKit runs rootless in `as0-builds` on dedicated nodes with `seccompProfile: Unconfined`. This is a deliberate trade-off for build compatibility, but these nodes represent a higher-privilege environment. A build-time escape from a compromised image could access the ECR credentials and node metadata.

---

## Competitor Analysis: What Other Platforms Use

### AI Agent Platforms

| Platform | Runtime | Cold Start | Browser | Snapshot | GPU | Notes |
|---|---|---|---|---|---|---|
| **Anthropic Managed Agents** | gVisor container | Not stated | No | No | No | Sandbox per tool call, not per session — 60% P50 / 90% P95 TTFT improvement. Credentials held outside sandbox in a vault. $0.08/session-hr. |
| **OpenAI Codex Cloud** | OCI containers (backing undisclosed) | Not stated | Limited | No | No | Network off by default during agent phase. Setup-phase secret injection, then secrets removed. 12hr container cache TTL. |
| **Azure AI Foundry Hosted Agents** | Hypervisor VM + container | ~seconds | Via Toolbox | Via session persistence | No (preview) | Full VM isolation per session. 30-day session state; `$HOME` persists across compute deprovisioning. Per-agent Entra identity. RBAC via Key Vault. |
| **LangGraph Cloud / Deep Agents** | Kubernetes containers | Not stated | No | App-level only (`interrupt()`) | No | Strongest orchestration/state model. `interrupt()` pauses for human approval and resumes from exact checkpoint. March 2026 security disclosure exposed files/secrets via framework flaws — no additional container hardening. |
| **Ona (fmr. Gitpod)** | Container + Kata Containers (opt-in microVM) | Not stated | Via workspace | No | Yes | Rebranded from Gitpod in Sept 2025; pivoted from dev environments to AI agent infrastructure. Supports workspaces up to 896 vCPU / 12TB RAM. Kata Containers available as RuntimeClass. |

### Execution Infrastructure Platforms

| Platform | Runtime | Cold Start | Browser | Snapshot | GPU | Notes |
|---|---|---|---|---|---|---|
| **E2B** | Firecracker microVMs | ~150ms | Yes (Chromium) | Yes (pause/resume) | No | 27-tool complete virtual computer per sandbox. Pre-warmed snapshot pool. Used by Manus, Perplexity. 24hr session max. |
| **Modal** | gVisor (systrap) | Sub-second | No | Snapshot primitives | Yes (H100 $3.95/hr) | Syscall interception, not VMs. GPU via `nvproxy`. 20,000+ concurrent. Used by Lovable, Quora. Python-first. |
| **Docker Sandboxes** | Custom VMM (KVM/Hyper.framework/WHP) | Near instant | Yes (full Docker) | No | No | Private Docker daemon per VM. Declarative policy set before agent runs; agent cannot self-modify its security boundary. Cross-platform (macOS/Windows/Linux). |
| **Vercel Sandbox** | Firecracker | Sub-second | No | No | No | GA April 2026. Up to 5hr sessions. $0.128/vCPU-hr, idle not billed. |
| **Fly.io Machines** | Firecracker | Sub-second | No | Yes (~300ms) | No | Persistent stateful VMs with checkpoint/restore. |
| **AWS Lambda/Fargate** | Firecracker | 125ms | No | Yes (~28ms w/ SnapStart) | No | Trillions of invocations/month. Jailer applies seccomp + cgroups to the VMM process itself. |
| **Google Cloud Run Gen 2** | MicroVMs | +100–500ms vs Gen 1 | No | No | No | Migrated from gVisor (Gen 1) due to syscall compatibility gaps. |
| **Daytona** | Docker (default), Kata (optional) | Sub-90ms | No | No | No | Default is containers — weaker isolation than competitors unless Kata is explicitly enabled. |
| **Morph** | MicroVMs | Sub-300ms | No | No | No | Purpose-built for AI coding agents. 4-layer isolation model. |
| **Northflank** | Firecracker/gVisor/Kata/Cloud Hypervisor | Seconds | No | No | Yes (H100 $2.74/hr) | Multi-runtime BYOC. 2M+ isolated workloads/month. Most cost-efficient in independent benchmarks. |

**Industry direction is clear:** every platform that built purpose-built AI agent sandboxing since 2024 chose microVMs or equivalent hardware isolation, not enhanced containers. The main exceptions are Modal (gVisor — strong but not hardware-level) and Anthropic Managed Agents (also gVisor). Notably, Microsoft Azure AI Foundry chose full hypervisor VMs for their agent platform. LangGraph is the outlier relying on containers without additional isolation — and had a significant security disclosure in March 2026.

---

## Technology Deep-Dive

### Firecracker

Rust-based VMM using Linux KVM. ~83,000 lines of Rust vs QEMU's ~2M lines of C. Minimal device model (6 virtual devices only — no GPU passthrough, no PCIe, no USB). Each microVM gets a dedicated kernel. The Jailer applies seccomp-bpf + cgroups + chroot + privilege-dropping to the Firecracker process itself as a second layer.

- Startup: 125ms standard, ~28ms with snapshot-restore
- Memory overhead: <5 MiB per instance
- Creation rate: up to 150/second per host
- Production scale: AWS Lambda (trillions of invocations/month)
- Escape requires a hypervisor CVE — $250K–$500K exploit market class

GPU passthrough is not supported by design. Running GPU workloads on Firecracker requires VIRTIO-GPU (software rendering only) or the workload runs on a separate non-sandboxed GPU node.

### gVisor

Google's user-space application kernel written in Go. Intercepts all syscalls in a Go Sentry process — never passes syscalls through to the host kernel. Host kernel syscall surface reduction from 450+ to 53–68 calls.

- Startup: ~50–100ms
- CPU overhead: 2.2× minimum on syscall-heavy code, up to 216× on some filesystem operations
- GPU support: `nvproxy` — CUDA syscall interception in memory-safe layer (Modal's approach)
- Compatibility gaps: missing syscalls cause failures for eBPF, io_uring, some ioctls — this is why GCP Cloud Run Gen 2 moved to microVMs
- NOT full kernel isolation: gVisor is still a host process; escape requires compromising both Sentry and host kernel simultaneously

### Kata Containers

OCI-compliant runtime that wraps each container/pod in a lightweight VM. From Kubernetes' perspective it is a `RuntimeClass` — no application changes. Pluggable VMM backend: QEMU, Firecracker, Cloud Hypervisor. Each container gets a dedicated kernel, process space, and network stack.

- Startup overhead: 150–300ms (Firecracker backend is fastest)
- Integrates directly with containerd/CRI-O and Kubernetes RuntimeClass
- Recently integrated into kubernetes-sigs/agent-sandbox (Google-led AI agent infrastructure project)
- Used in production by IBM Cloud and AWS EKS bare-metal

### Kubernetes User Namespaces (v1.36 GA)

User namespaces remap container root (UID 0) to an unprivileged host UID via ID-mapped mounts (Linux 5.12+). A process that is root inside the container has only unprivileged host access if it escapes.

- Enabled by default in Kubernetes v1.33+, GA in v1.36
- Single pod spec field: `hostUsers: false`
- Explicitly mitigated: CVE-2025-31133, CVE-2025-52565, CVE-2025-52881 (all three 2025 runc escape CVEs)
- Does NOT provide kernel isolation — the kernel is still shared. Reduces blast radius of escape; does not prevent kernel exploits
- ID-mapped mounts make startup O(1) regardless of volume size — no longer a performance concern
- Linux-only. No image changes required.

---

## Isolation Strength Ranking

```
Strongest ──────────────────────────────────────────────────────

  Firecracker / Cloud Hypervisor
    Dedicated kernel per VM. CPU-enforced hardware boundary (Intel VT-x / AMD-V).
    Escape requires a hypervisor CVE. $250K–$500K exploit market class.

  Kata Containers
    Same hardware boundary via OCI-compliant VMM wrapping.
    Container tooling compatibility with VM-level isolation.

  gVisor (Sentry + KVM mode)
    Userspace kernel — no syscall passthrough to host.
    Escape requires compromising both Sentry and host kernel simultaneously.
    Not hardware isolation; still a host process.

  Rootless containers + user namespaces + seccomp + AppArmor
    Reduces blast radius. Root inside = unprivileged outside.
    Does NOT prevent kernel exploits. Eight CVEs defeated this layer in 2024–2025.

  Standard OCI containers (current Astro)
    No hardware boundary. Direct kernel attack surface shared across tenants.

Weakest ──────────────────────────────────────────────────────
```

---

## Capabilities Unlocked by MicroVMs

Running agents inside a full VM enables a qualitatively different set of capabilities, not just stronger isolation. This is what makes microVMs a product decision as much as a security one.

### Browser Automation (Headful)

Inside a VM, agents can run a real Chromium or Firefox with full sandbox flags (`--no-sandbox` is **not** required — the VM is the outer isolation boundary). Combined with a virtual framebuffer (Xvfb) or VNC, agents can drive GUI browsers with JavaScript execution, session cookies, file downloads, and real rendering.

In containers on a shared host, running headful Chromium requires `--no-sandbox`, which disables Chromium's own internal sandboxing — a meaningful security regression. This means browser-using agents in containers are either less capable or less safe.

Manus's E2B-based architecture includes Chromium as one of 27 tools available inside each Firecracker VM. Playwright MCP exposes ~15 browser actions to LLMs via MCP and supports a noVNC mode for real-time agent observation.

### Desktop GUI Access

VMs provide a full display stack (X11 or Wayland + window manager + VNC/RDP/noVNC). Agents can drive native GUI applications not reachable via CLI or web APIs — file managers, desktop IDEs, legacy GUI tooling. Containers cannot provide this without installing a full desktop stack on the shared host with elevated privileges.

### Full Filesystem with Snapshotting and Branching

VMs provide block-device-backed filesystems with true snapshotting:

- **Firecracker full snapshot:** memory + disk state captured simultaneously, restored in ~28ms
- **Copy-on-write fork (Zeroboot pattern):** 0.79ms P50, 265KB overhead per fork, 1,000 parallel branches achievable in ~815ms
- **Azure Foundry pattern:** `$HOME` and `/files` persist across compute deprovisioning and resume on next request — zero-cost idle state

This enables `fork → explore → commit` agent execution patterns: snapshot the VM at a decision point, restore into N independent branches to explore different approaches in parallel, commit the best result. Useful for tree-of-thoughts, best-of-N sampling, and reflexion loops at the system level.

Containers cannot do this. `docker commit` captures the filesystem only, not in-memory process state. CRIU exists for container checkpoint/restore but is slow, fragile under load, and breaks on open network connections.

### Long-Running Sessions Across Async Gaps

VM pause/snapshot allows cost-efficient long-running sessions: compute charges only when active, state preserved across idle periods. Azure Foundry supports sessions up to 30 days this way. Fly.io "Sprites" (persistent Firecracker VMs) resume in ~300ms.

LangGraph's `interrupt()` primitive achieves async-gap pausing but only for application state the developer explicitly checkpoints — it requires the agent framework to be built around it. VM-level snapshotting requires nothing from the application.

### Arbitrary Package and Kernel Tool Installation

Inside a VM, the agent has root on a disposable kernel. This enables:

- `apt install`, `pip install`, `cargo install` without risk to other tenants — the VM is throwaway
- Kernel module loading (`insmod`/`modprobe`) — impossible in a shared-kernel container without `CAP_SYS_MODULE`, which allows arbitrary kernel code execution on the host
- eBPF programs — requires `CAP_SYS_ADMIN` in containers (near-root on the shared host); inside a VM it's contained to the VM's kernel
- `strace`/`ptrace` — requires `CAP_SYS_PTRACE` in containers, risks other tenants in the same PID namespace; inside a VM it is fully contained
- `perf`/`bpftrace` for performance profiling — hardware counters via `perf_event_open`, kernel tracing, all inside the VM's kernel

OpenAI Codex addresses the package installation problem by allowing it only during a pre-execution setup phase before the agent runs, then revoking network access. This prevents runtime installation but also prevents agents from adapting their environment mid-task.

### Full Network Services and Port Forwarding

Agents inside a VM can run arbitrary local network services (web servers, databases, message brokers) on any port, with full TCP/UDP stack isolated from the host and other tenants. The hypervisor handles port forwarding to external clients. E2B and Ona/Gitpod expose sandbox ports via workspace URLs for web preview workflows.

In containers, service exposure requires orchestrator-managed port mappings and shares the host network namespace (modulo network namespaces). Running multiple services requires a process supervisor and is architecturally unclean.

### Nested Containers (Private Docker Daemon)

Docker Sandboxes gives each agent VM its own private Docker daemon, enabling full `docker build` / `docker run` / `docker compose` inside the sandbox without socket mounting or host-level privileges. Agents can build and run containers inside their VM.

In containers, this requires Docker-in-Docker (`--privileged`) or socket mounting (`/var/run/docker.sock`), both of which break tenant isolation completely.

### Observability Outside the Agent's Reach

In a VM, monitoring occurs at the hypervisor level. The agent cannot disable, tamper with, or detect the monitoring infrastructure — it is architecturally outside the VM's trust boundary. In a container, a root-equivalent process can manipulate its own logging, disable seccomp handlers, or probe the monitoring stack.

### What Containers Fundamentally Cannot Offer

| Capability | OCI Container | MicroVM |
|---|---|---|
| Dedicated kernel | No — shared host kernel | Yes — hardware-enforced by CPU |
| Kernel escape cost | 8 CVEs in 18 months; $10K bounty class | Requires hypervisor CVE; $250K–$500K bounty class |
| Headful browser with full sandbox | Requires `--no-sandbox` (weakens browser) | Full Chromium sandbox flags |
| Live memory + disk snapshot | No (CRIU fragile, breaks on network) | Yes — Firecracker ~28ms |
| Sub-ms CoW fork for parallel branching | No | Yes (Zeroboot: 0.79ms P50) |
| Kernel module loading | No (`CAP_SYS_MODULE` → host kernel exposure) | Yes — contained to VM kernel |
| eBPF programs | No (`CAP_SYS_ADMIN` → near-root on host) | Yes — contained to VM kernel |
| Private Docker daemon | Requires `--privileged` or socket mount | Yes — standard capability |
| Long-running idle without cost | Container must stay running | VM snapshots; resume on demand |
| Monitoring outside agent's reach | No — agent can manipulate its environment | Yes — hypervisor-level |

---

## Options

### Option 1 — Harden Containers (Stay on OCI, Improve Isolation)

Keep the existing OCI container model on EKS but systematically close the gaps in the current security posture.

**Enable user namespaces (`hostUsers: false`)**
Kubernetes v1.33+ enables this by default; EKS 1.33+ supports it. Add `hostUsers: false` to all agent pod specs. This remaps container root to an unprivileged host UID and directly mitigates all three 2025 runc escape CVEs. Zero application changes, minimal overhead due to ID-mapped mounts.

**Enforce egress NetworkPolicies for all agent namespaces**
Current policies cover private store access. Add a default-deny-egress policy per agent namespace, then allowlist only required destinations. Add DNS-level filtering (CoreDNS RPZ or a DNS proxy sidecar) to prevent DNS tunneling — the AWS AgentCore lesson. This is critical regardless of isolation model chosen.

**Migrate build nodes to rootless BuildKit with user namespace isolation**
Currently `seccompProfile: Unconfined` on build nodes. Enable user namespaces on build pods. Accept minor compatibility trade-offs in return for dramatically reduced blast radius on build escapes.

**Harden seccomp profiles per component type**
`RuntimeDefault` is a baseline. Generate and apply fine-grained seccomp profiles per agent container (using `seccomp-bpf` profiling in staging). Reduces kernel attack surface without microVM overhead.

**Node isolation per tenant (for high-value customers)**
Pin sensitive deployments to dedicated node groups with a node-per-tenant policy. Container escape stays within the customer's own node. Expensive (no bin-packing) but simple to implement with node affinity and taints.

**What this doesn't solve:** A sufficiently sophisticated kernel exploit still reaches the host. An LLM agent can still probe and enumerate its environment. Any new runc CVE is a direct threat until patched. This option is appropriate for agents with bounded, predictable tool use — not for agents that execute arbitrary user-supplied code.

---

### Option 2 — Switch to MicroVMs

Replace the OCI container runtime for agent workloads with microVM-backed execution. The recommended path is **Kata Containers with a Firecracker backend**, deployed as a Kubernetes RuntimeClass.

**Why Kata + Firecracker over raw Firecracker:**
Kata is OCI-compliant — it integrates with containerd and Kubernetes without changing the deployment spec, the CLI, or how images are built. The `deployment/v1` spec's `target.runtime: kubernetes` continues to work. The only change is adding a `runtimeClassName: kata-firecracker` field to agent pod specs (or making it the default RuntimeClass for agent namespaces).

**Why now:**
- Every purpose-built AI agent platform launched since 2024 has chosen microVMs
- Eight container escape CVEs in 2024–2025 are not a statistical anomaly — they reflect the attack surface inherent to shared-kernel containers
- Kubernetes v1.36 and EKS bare-metal support Kata natively
- The `target.runtime` seam in the spec exists as if anticipating this
- Cognition's experience building Devin, and Docker's own migration in late 2025, both independently reached the same conclusion: shared-kernel containers cannot safely run autonomous agents

**What changes:**
- Provision EKS bare-metal nodes (required — Kata uses KVM, not available on virtualized EC2 instances). AWS supports this via metal instance types (`m5.metal`, `c5.metal`, etc.)
- Install Kata + Firecracker via the `kata-deploy` DaemonSet on agent node groups
- Add a `kata-firecracker` RuntimeClass to the cluster
- Set `runtimeClassName: kata-firecracker` in agent pod template (one field in `template.go`)
- No changes to agent images, CLI, spec format, or registry

**GPU workloads:**
Firecracker does not support GPU passthrough. GPU agents either run on a separate non-Kata node group (existing behavior, flagged as lower isolation), or use gVisor's `nvproxy` via a KVM-mode RuntimeClass on GPU nodes. This is a real trade-off. Modal's approach (gVisor + nvproxy for GPU) is the strongest option for GPU-accelerated agents.

**Performance trade-offs:**
- Pod startup adds 150–300ms with Kata/Firecracker (snapshot-restore brings this to ~50ms with pre-warmed pools)
- No memory overcommit — each microVM has a hard memory reservation. This reduces bin-packing density and increases per-agent cost vs containers
- Cold-start matters less for long-running agent deployments (K8s Deployments) than for ephemeral sandboxes. The overhead is a one-time cost per agent startup, not per request

**Cost implication:**
Bare-metal EC2 is more expensive than virtualized EC2 and eliminates memory overcommit. Based on Northflank's analysis, expect 2–3× higher infrastructure cost per agent slot vs containers on virtualized nodes. This is partially offset by simpler security tooling and eliminating the need for EDR/runtime threat detection per node.

---

### Option 3 — Keep Containers, Offer MicroVMs as a Premium Tier

Operate the current OCI container model as the default tier with the Option 1 hardening applied. Introduce a microVM-backed execution tier as an opt-in for workloads requiring stronger isolation guarantees.

**Positioning:**

| Tier | Runtime | Isolation | Use case |
|---|---|---|---|
| Standard | Hardened OCI containers (user namespaces, tight egress, seccomp) | Namespace-level | Internal agents, trusted code, model/knowledge workloads |
| Secure | Kata/Firecracker (bare-metal nodes) | Hardware VM | Untrusted code execution, customer-facing agents, arbitrary tool use |

**Spec integration:**
Add an optional `isolation` field to `deployment/v1`:
```yaml
target:
  runtime: kubernetes
  isolation: vm  # default: container
```
The server resolves `isolation: vm` to a pod spec with `runtimeClassName: kata-firecracker` on the bare-metal node group. No other changes to the agent image or deployment pipeline.

**Rollout order:**
1. Apply Option 1 hardening immediately — user namespaces, egress NetworkPolicies, DNS filtering. Low risk, high impact.
2. Provision a small bare-metal node group and validate Kata/Firecracker on EKS.
3. Run the `as0-builds` namespace on the VM tier first — it currently runs `seccompProfile: Unconfined` and is the highest-risk environment in the cluster.
4. Offer `isolation: vm` as a beta feature to select customers.
5. Make `isolation: vm` the default for new agent deployments once stable.

**Why this is the recommended option:**
It delivers immediate security improvements (Option 1) without blocking on infrastructure changes, while building toward the correct long-term architecture (Option 2). It also creates a clear product differentiation opportunity and maps to how E2B and Northflank price their tiers.

---

## Decision Factors Summary

| Factor | Option 1 (Harden containers) | Option 2 (Switch to VMs) | Option 3 (Containers + VM tier) |
|---|---|---|---|
| Implementation speed | Fast (days–weeks) | Slow (weeks–months) | Medium |
| Security improvement | Moderate — reduces blast radius, does not eliminate kernel exposure | Strong — hardware boundary, industry standard for agent execution | Moderate immediately, strong at tier graduation |
| Cost impact | Minimal | High (bare-metal, no overcommit) | Incremental |
| Agent compatibility | Full | Full (OCI-compatible) | Full |
| GPU support | Full | Partial (separate node group) | Per tier |
| Operational complexity | Low | High (bare-metal node management, Kata version pinning) | Medium |
| Industry alignment | Behind curve | At parity with E2B, Vercel, Modal | Pragmatic path to parity |
| Spec changes required | None | None | Minor (`isolation` field) |

---

## Recommendation

**Implement Option 3.**

Start with the Option 1 hardening immediately — user namespaces (`hostUsers: false`) and strict egress NetworkPolicies with DNS filtering are low-risk, high-impact changes that fix the most realistic near-term threats and should be done regardless of the VM decision.

In parallel, provision bare-metal EKS nodes and validate Kata/Firecracker. Start with the build namespace (`as0-builds`) since it already runs with `seccompProfile: Unconfined` and is the current highest-exposure environment. Once validated, introduce the `isolation` field in the spec and offer VM execution as a beta tier. The `target.runtime` seam in the spec was clearly designed to support exactly this kind of evolution.

Long-term direction is Option 2 — microVM-backed execution for all agent workloads is where the industry has landed and where the security model needs to go for arbitrary agent execution.
