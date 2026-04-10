# Knowledge Store as a First-Class Citizen

## Problem

Knowledge bases today are lifecycle-tied to agent deployments. Two things this prevents:

1. **Sharing** — two agents that need the same vector store must each provision their own copy, duplicating infrastructure and data.
2. **Bring-your-own** — users with existing databases (Postgres, Qdrant, Pinecone, etc.) cannot onboard them. The platform assumes it creates everything.

---

## Core Concept

A `KnowledgeStore` becomes a standalone account-level entity with its own lifecycle — created, managed, and deleted independently of any agent.

Each store gets a stable resource identifier — an **ARN** (Astro Resource Name):

```
arn:knowledge:acme:postgres-main
arn:knowledge:acme:shared-qdrant
```

The format is `arn:{type}:{account}:{name}`. Flat, colon-separated, fully qualified. `knowledge` is the first resource type to use this scheme.

---

## Two Modes

### Managed

The platform provisions and operates the database in the account namespace. The store has its own lifecycle (`provisioning`, `ready`, `error`) independent of any agent deployment. Multiple agents can bind to it simultaneously.

### External

The user has an existing database — for example, a Postgres instance on RDS. They onboard it by providing connection details. The platform stores these encrypted under an ARN. No infrastructure is created; the platform is only a credential broker.

---

## User Experience: Postgres Example

### Managed Postgres

A user wants a shared Postgres instance that multiple agents can use. They create it:

```
ast knowledge create \
  --name postgres-main \
  --provider postgres \
  --persistent \
  --storage 20Gi
```

The platform provisions it and assigns the ARN `arn:knowledge:acme:postgres-main`. It appears under a **Knowledge** section in the UI and CLI, independent of any agent. When deploying an agent, the user binds a knowledge entry to this ARN — the platform wires in the already-running store rather than provisioning a new one.

### External Postgres (Bring-Your-Own)

A user has a Postgres instance on RDS. They onboard it:

```
ast knowledge connect \
  --name postgres-prod \
  --provider postgres \
  --host db.example.com \
  --port 5432 \
  --database vectors
```

The platform prompts for credentials, stores them encrypted, and assigns the ARN `arn:knowledge:acme:postgres-prod`. It appears alongside managed stores in the Knowledge section, badged as **External**. The platform never touches the underlying database. When an agent is deployed and bound to this ARN, the real connection credentials are injected as environment variables.

---

## Private Connectivity

The external mode above assumes the database has a reachable endpoint — a public host or one accessible from Astro's network. In practice, most production databases are not publicly accessible. They live inside private VPCs, behind firewalls, in corporate networks, or on-premises. Providing a host and credentials is not enough when the host itself is unreachable.

This is not an edge case. It is the default for any serious production database.

To onboard a private database, the platform needs a connectivity layer that bridges Astro's cluster to the user's private network without requiring the user to open inbound firewall rules or expose the database to the internet. Two options are offered:

**PrivateLink** — cloud-native private networking (AWS PrivateLink, GCP Private Service Connect, Azure Private Link). The database is exposed via a load balancer and endpoint service within the user's cloud account; Astro creates a VPC endpoint in its own cluster VPC. Traffic never leaves the cloud provider's backbone. Best for high-throughput, single-cloud deployments in regulated environments.

**Connector Agent** *(experimental)* — a small container (`ast-connect`) deployed inside the user's private network. It establishes an outbound encrypted tunnel to Astro's fleet endpoint using the existing QUIC infrastructure (the same system that powers `ast connect`). No inbound firewall rules required. Works across any cloud, any region, on-premises, and multi-cloud. Best for most users and for cross-cloud scenarios where PrivateLink cannot reach.

See [knowledge-store-private-connectivity.md](./knowledge-store-private-connectivity.md) for the full design of both options, including protocol details, performance characteristics, and cross-cloud support.

---

## Key Decisions

**ARN over name matching.** Name matching is implicit and fragile — a coincidental name collision silently redirects a deployment. An ARN is an explicit, intentional binding. You choose to bind; nothing happens by accident.

**Flat ARN format.** `arn:knowledge:acme:postgres-main` — type, account, and name in a single colon-separated string. No hierarchy, no region, no verbosity.

**Entry name and store name are independent.** The knowledge entry name an agent uses determines the env var prefix it sees. The ARN is the actual store. They can differ — one agent calls it `db`, another calls it `main-db`, both bound to `arn:knowledge:acme:postgres-main`.

**Managed stores provision on creation, not on deploy.** The store is ready before any agent references it. Deploy-time failures are not caused by database provisioning.

**External stores are opaque.** The platform stores credentials and injects them. It does not manage schema, indices, or data.

**Provider mismatch is a hard error.** If an agent expects a Postgres connection but the bound ARN points to a Qdrant store, the deploy is rejected.
