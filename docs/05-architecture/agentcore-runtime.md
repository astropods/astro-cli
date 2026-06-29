# Running Astro Agents on AWS Bedrock AgentCore

Integration notes and open questions — shared with the AWS Bedrock AgentCore team.

Astro is a platform for deploying and running AI agents. Users package an agent as a container, and we run it alongside managed models, knowledge / vector stores, tool integrations, messaging, identity, and observability. Today agents run as containers on Amazon EKS. We are evaluating **AgentCore Runtime as an alternative place to run the agent's container**, keeping the rest of the platform as-is. This note describes how we'd integrate it and the questions we'd like AWS's input on. Where we state how AgentCore behaves, it's our current understanding — please correct anything wrong.

---

## How Astro runs an agent today

Each deployment is a **private EKS namespace** containing:

- the **agent container** — always-on, and
- the services it depends on — knowledge / vector stores, self-hosted models, tool integrations, a messaging component (Slack / web chat), and an OpenTelemetry collector.

The agent reaches its dependencies over **private in-cluster DNS**. Inbound web traffic arrives through an ingress with OIDC enforced at the load balancer. Durable state lives on an attached volume. Everything is co-located and privately addressed.

---

## AgentCore Runtime, as we understand it

A serverless, per-session runtime: we ship a container that serves `POST /invocations` and `GET /ping` on port 8080; AWS handles inbound auth (SigV4 / JWT-OIDC), per-session isolation, and scaling; callers reach the agent through the `InvokeAgentRuntime` API rather than a URL. Memory, Identity, Gateway (tools), and Observability are available as managed primitives. (We're targeting Runtime with a bring-your-own container, not the higher-level Harness.)

```mermaid
flowchart LR
  subgraph container["Agent container (ARM64) — port 8080"]
    INV["POST /invocations<br/>JSON in → JSON or SSE out"]
    PING["GET /ping<br/>→ Healthy | HealthyBusy"]
    WS["optional: WebSocket /ws"]
  end
  RT["AgentCore Runtime"] -->|invoke| INV
  RT -->|health / idle detection| PING
  RT -->|streaming sessions| WS
```

The properties that shape our integration:

- **Invoke-only** — no stable inbound URL; reach is `InvokeAgentRuntime(arn, …)`.
- **No co-located sidecars** — the runtime hosts our container; there is no second container alongside it.
- **No durable filesystem** — per-session scratch only; durable state goes to Memory or an external store.
- **Scales to zero** — consumption-based; idle sessions reaped via `/ping`.

---

## Integration model: relocate only the agent container

The key decision: **EKS and AgentCore co-exist within a single deployment**. We keep the namespace and every surrounding service exactly as today; only the **agent's own container** moves to AgentCore. Wherever the agent runs, it connects back to its dependencies in EKS.

```mermaid
flowchart LR
  subgraph eks["Amazon EKS — private namespace (unchanged)"]
    DEPS["knowledge / vector stores<br/>self-hosted models · tools<br/>messaging · OTel collector"]
  end
  AGENT{{"Agent container<br/>EKS pod today — OR — AgentCore proposed"}}
  AGENT -->|"must reach over private DNS / VPC"| DEPS
```

This confines the runtime choice to the single component that executes agent code; the surrounding platform (knowledge, models, tools, messaging, telemetry, ingress, auth policy) is unchanged. Per capability:

| Capability | Today (EKS) | On AgentCore | Note |
|---|---|---|---|
| Compute | always-on pod | serverless runtime | the swap |
| Reach | private service DNS + ingress | `InvokeAgentRuntime` (ARN) | URL → invoke |
| Messaging | co-located sidecar | no sidecar | must become a standalone service |
| Inbound auth | OIDC at the load balancer + our grants | Identity (SigV4 / JWT-OIDC) | policy stays ours, mechanism per-runtime |
| Durable state | attached volume | Memory or external | the volume is EKS-only |
| Models | self-hosted **or** model gateway | Bedrock **or** gateway | self-hosted is EKS-only |
| Knowledge / vector | self-hosted **or** managed | managed only | self-hosted needs private reach |
| Tools | sidecar services | Gateway (MCP) / external MCP | sidecar is EKS-only |
| Observability | OTel collector → our backend | OTel → CloudWatch or external | portable if the collector is reachable |
| Ingestion (batch/cron) | K8s Jobs / CronJobs | EventBridge / Lambda | EKS-only as built |

Two consequences follow:

- **Messaging must move out of the agent.** Today our messaging component is co-located with the agent. If only the agent relocates, messaging becomes a standalone service the agent connects to — or that invokes the agent.
- **The agent must reach a private VPC from outside it.** This is the crux, and the subject of the questions below. Managed/external dependencies (managed knowledge, the model gateway) already work because they're addressed by a public or PrivateLink URL; the self-hosted in-namespace dependencies are the problem.

---

## The agent-runtime interface

Concretely, the agent-execution step becomes a small pluggable interface. The current Kubernetes path is one implementation; AgentCore is another. The interface covers only the agent box — everything else stays where it is.

```go
// AgentRuntime runs the single agent-execution component. Everything else in the
// namespace is applied as it is today, regardless of which implementation runs.
type AgentRuntime interface {
	ApplyAgent(ctx context.Context, in AgentSpec) (AgentHandle, error)
	TeardownAgent(ctx context.Context, deploymentID string) error
	AgentStatus(ctx context.Context, deploymentID string) (AgentStatus, error)
	Capabilities() Capabilities
}
```

The supporting types: `AgentSpec` is the agent container's own needs plus the resolved addresses it must reach; `AgentHandle` is the set of ways to reach the box (one entry per surface the agent serves — http, gRPC, web… — each with its protocol and an in-cluster plus optional external address on EKS, a single runtime ARN on AgentCore); `AgentStatus` is the box's live state with a runtime-agnostic `Health` plus EKS-only replica/phase detail.

```go
// input: the agent container's own needs + the resolved addresses it must reach.
type AgentSpec struct {
	DeploymentID string            // stable id; namespace suffix on EKS
	Image        string            // resolved OCI image ref
	Port         int32             // container port the agent serves
	Env          map[string]string // fully resolved: dependency URLs, creds, auth token, OTEL endpoint
	Volume       string            // durable-state mount path; "" = none. EKS: volume, AgentCore: ignored
}

// output: how the platform reaches the agent box.
type AgentHandle struct {
	// EKS: one entry per surface the agent serves (http, grpc, web, …)
	Endpoints map[string]AgentEndpoint
	// AgentCore: a single runtime ARN reaches the container; no per-surface URLs
	Invoke string
}

type AgentEndpoint struct {
	Protocol string // "http" | "grpc"
	Internal string // in-cluster address, e.g. "<svc>.<ns>.svc:8080"
	External string // ingress URL if exposed, else ""
}

// live state of the agent box.
type AgentStatus struct {
	Health   string // "ready" | "progressing" | "degraded" | "unavailable" | "serverless"
	Ready    int32  // ready replicas — EKS only (0 on AgentCore)
	Replicas int32  // total replicas — EKS only (0 on AgentCore)
	Phase    string // pod phase      — EKS only ("" on AgentCore)
}
```

`Capabilities()` advertises what the box supports, so the control plane checks the spec against the target up front and rejects one that hard-requires something it can't honor — before deploying, rather than failing mid-deploy.

```go
// Capabilities is what an AgentRuntime can give the agent box.
type Capabilities struct {
	PersistentDisk bool // durable volume                 — EKS: true, AgentCore: false
	Replicas       bool // operator-pinned replica count  — EKS: true, AgentCore: false
	WebIngress     bool // agent-served web UI at a URL    — EKS: true, AgentCore: false (invoke-only)
}
```

The EKS implementation returns all `true` (it accepts every spec it does today); the AgentCore implementation returns all `false` — a spec that needs a durable volume, pins replicas, or serves its own web UI is rejected up front with a clear reason.

---

## Open questions for the AgentCore team

1. **VPC attachment.** Can an AgentCore Runtime run attached to (or peered with) a customer-owned VPC so it can reach private resources (in-EKS services, RDS) by private IP / DNS? Or is outbound limited to the public internet / NAT?
2. **Private DNS resolution.** Can the runtime resolve private Route 53 zones or in-cluster service names, or must every dependency be exposed as a public or PrivateLink endpoint?
3. **PrivateLink path.** Is there a supported pattern to reach a service fronted by a VPC endpoint / PrivateLink from inside the runtime, and are there per-session connection limits?
4. **Persistent outbound connections.** Can the agent hold a long-lived outbound connection (e.g. a gRPC stream to a messaging service) for the duration of a session, or is networking torn down between `/invocations` calls? This decides whether messaging can keep an "agent dials in" model or must invert to "platform invokes the agent."
5. **Private inbound invoke.** Can a component inside the VPC call `InvokeAgentRuntime` over a private path (PrivateLink to the `bedrock-agentcore` endpoint), or only via the public endpoint? Needed if messaging stays in-cluster and invokes the agent.
6. **Egress controls.** What outbound filtering / allowlisting is available — can we restrict egress to specific destinations (DNS included) and deny the rest?
7. **Stable egress identity.** Is there a stable source identity (IP or PrivateLink principal) the VPC side can authorize, so private dependencies can authenticate the runtime's connections?
8. **Per-invocation latency.** For an agent that makes many calls to private dependencies within a single invocation, what is the added round-trip of egressing the runtime back into a VPC, and is connection reuse available across the session?
9. **Hosting agent-served web UIs.** Some agents serve their own browser-facing web app, reached today at a stable HTTPS URL through our ingress. Can AgentCore expose an agent's HTTP server as a browser-reachable site — stable URL, standard browser auth (cookies / redirects), static assets, WebSocket — or is the only inbound path `InvokeAgentRuntime`, which assumes a programmatic caller rather than a browser? If direct browser hosting isn't supported, what's the recommended pattern for a customer-facing web UI backed by an AgentCore agent?

---

## References

- [AgentCore HTTP protocol contract](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/runtime-http-protocol-contract.html)
- [What is AgentCore](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/what-is-bedrock-agentcore.html)
- [Inbound & Outbound Auth](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/runtime-oauth.html)
- [Observability](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/observability-configure.html)
