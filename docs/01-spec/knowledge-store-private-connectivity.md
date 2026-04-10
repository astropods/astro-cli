# Knowledge Store Private Connectivity

External knowledge stores are often not publicly accessible — they live inside private VPCs, corporate networks, or on-premises infrastructure. This document covers two connectivity options the platform offers: **PrivateLink** (cloud-native private networking) and a **Connector Agent** (reverse tunnel over QUIC).

Both options are transparent to agents at runtime. The store is still `arn:knowledge:acme:postgres-prod`; the connectivity mechanism is an infrastructure detail.

---

## Option A: PrivateLink

### Concept

Cloud providers offer a native private networking primitive that exposes a service in one VPC as a reachable endpoint in another VPC, without traversing the public internet. AWS calls it **PrivateLink**, GCP calls it **Private Service Connect**, Azure calls it **Private Link**.

The user exposes their database via a load balancer, registers it as an endpoint service, and shares it with Astro. Astro creates a VPC endpoint in its cluster VPC. The database is then reachable by agents over a private IP — no relay, no tunnel, native TCP throughput.

```
Agent Pod (Astro VPC)
  └─→ VPC Interface Endpoint (private IP in Astro VPC)
         └─→ [cloud provider backbone — no public internet]
                └─→ NLB (User VPC)
                       └─→ Private DB (db.internal:5432)
```

### AWS PrivateLink Setup

#### User-side

1. Create a **Network Load Balancer (NLB)** in the same VPC as the database. Add a TCP listener on the database port (e.g. 5432) targeting the database instance or RDS endpoint.

2. Create a **VPC Endpoint Service** from the NLB:
   - AWS Console → VPC → Endpoint Services → Create
   - Select the NLB
   - Under "Allowed principals", add Astro's AWS account ID (displayed in the platform UI)
   - Note the generated service name: `com.amazonaws.vpce.us-east-1.vpce-svc-0abc123...`

3. Provide the service name to Astro:
   ```
   ast knowledge privatelink attach \
     --store arn:knowledge:acme:postgres-prod \
     --provider aws \
     --service com.amazonaws.vpce.us-east-1.vpce-svc-0abc123 \
     --region us-east-1
   ```

#### Astro-side (automated after user runs the above)

1. Creates a VPC Interface Endpoint in Astro's cluster VPC targeting the user's endpoint service
2. Waits for acceptance — user's endpoint service may require manual approval or be set to auto-accept from Astro's account ID
3. Once accepted, the endpoint receives a private DNS name within Astro's VPC
4. Stores this DNS name as the resolved host for the store ARN
5. Updates store status to `ready`

#### What the agent sees

```
POSTGRES_HOST = vpce-0abc123-x.us-east-1.vpce.amazonaws.com
POSTGRES_PORT = 5432
```

No relay, no tunnel. Direct TCP to the user's NLB via the cloud provider's private backbone.

#### Acceptance flow

```
ast knowledge status arn:knowledge:acme:postgres-prod
→ Status: pending-acceptance
→ Action required: accept the endpoint connection request in your AWS Console
→ Endpoint service: com.amazonaws.vpce.us-east-1.vpce-svc-0abc123
```

Once accepted:
```
→ Status: ready
→ Endpoint: vpce-0abc123-x.us-east-1.vpce.amazonaws.com
```

### GCP Private Service Connect

1. User creates an **internal TCP load balancer** targeting the database
2. User creates a **Service Attachment** (Published Service) from the load balancer; provides Astro's GCP project ID as an allowed consumer
3. User provides the attachment URI: `projects/my-project/regions/us-central1/serviceAttachments/my-db`
4. Astro creates a **forwarding rule** (Consumer Endpoint) in its GCP project, obtaining a private IP
5. Private IP is injected as `POSTGRES_HOST` into bound agents

### Azure Private Link

1. User creates a **Private Link Service** from an internal load balancer, or enables Private Endpoint directly on an Azure-managed database (Azure Database for PostgreSQL supports this natively)
2. User provides the resource ID to Astro
3. Astro creates a **Private Endpoint** in its Azure VNet, obtains a private IP
4. Private IP is injected as `POSTGRES_HOST` into bound agents

### Cost Considerations

PrivateLink costs appear on both sides:
- **User's cloud bill**: endpoint service NLB costs + data processing fees
- **Astro's cloud bill**: VPC endpoint hourly fee + data transfer. Astro passes this through as a PrivateLink connectivity charge on the account.

AWS PrivateLink typical costs: ~$0.01/hr per endpoint + $0.01/GB data processed.

---

## Option B: Connector Agent *(Experimental)*

### Foundation: The Existing QUIC Infrastructure

The platform already has a production QUIC-based persistent connection system — `ast connect` — used by the CLI to maintain a long-lived stream to astro-server for device registration and command dispatch. It runs on UDP port 9092 via an AWS NLB with QUIC-LB connection ID routing, gRPC-over-QUIC with TLS 1.3 built in, and bidirectional streaming.

The connector agent reuses this entire transport stack. There is no new relay service, no new port, no new NLB. A new gRPC service (`TunnelService`) is added alongside the existing `ConnectService` on the same QUIC endpoint.

### Architecture

```
Agent Pod (Astro cluster)
  └─→ astro-server TunnelService (internal gRPC, TCP)
         └─→ QUIC streams (port 9092, same NLB as ast connect)
                └─→ ast-connect (user's VPC)
                       └─→ Private DB (db.internal:5432)
```

Two legs with the right protocol for each:
- **Agent → astro-server**: in-cluster gRPC over plain TCP. No QUIC needed; it is a cluster-internal call.
- **astro-server → connector**: QUIC streams over the existing fleet NLB. Each database connection from an agent opens one QUIC stream to the connector.

### Protocol

A new `TunnelService` is added to the proto alongside `ConnectService`.

```proto
service TunnelService {
  rpc Tunnel(stream TunnelClientMessage) returns (stream TunnelServerMessage);
}

message TunnelClientMessage {
  oneof payload {
    RegisterConnector register  = 1;
    TunnelHeartbeat   heartbeat = 2;
    StreamData        data      = 3;
    StreamClose       close     = 4;
  }
}

message RegisterConnector {
  string connector_id = 1;
  string store_arn    = 2;
  string target_host  = 3;
  int32  target_port  = 4;
}

message TunnelHeartbeat {
  int64 timestamp_unix = 1;
}

message StreamData {
  string stream_id = 1;
  bytes  payload   = 2;
}

message StreamClose {
  string stream_id = 1;
  string reason    = 2;
}

message TunnelServerMessage {
  oneof payload {
    RegisterConnectorAck register_ack = 1;
    OpenStream           open_stream  = 2;
    StreamData           data         = 3;
    StreamClose          close        = 4;
  }
}

message RegisterConnectorAck {
  bool   accepted = 1;
  string message  = 2;
}

message OpenStream {
  string stream_id = 1;
}
```

**Data plane flow for one database connection:**

1. Agent pod needs a DB connection → connects to astro-server's tunnel proxy address (e.g. `tunnel.astro-internal:5432`)
2. astro-server looks up the ARN → picks a healthy connector from the pool
3. Server sends `OpenStream { stream_id: "abc" }` to the connector over the QUIC bidi stream
4. Connector opens a TCP connection to `target_host:target_port`
5. Both sides exchange `StreamData` messages
6. Either side sends `StreamClose` when done

Multiple simultaneous database connections are multiplexed over the single QUIC bidi stream via independent `stream_id` values.

### Authentication

The connector uses a **connector token** — a signed JWT scoped to a single store ARN — sent as gRPC metadata (`authorization: Bearer <token>`), identical to how the CLI sends user JWTs for `ast connect`. No new auth machinery.

```
ast knowledge connector create \
  --store arn:knowledge:acme:postgres-prod \
  --name prod-vpc-connector
→ eyJ...

ast knowledge connector list --store arn:knowledge:acme:postgres-prod
→ NAME                STATUS     LAST SEEN
→ prod-vpc-connector  connected  3s ago

ast knowledge connector revoke prod-vpc-connector \
  --store arn:knowledge:acme:postgres-prod
→ Token revoked. Active tunnel dropped immediately.
```

### Connector Deployment

| Variable | Description |
|---|---|
| `AST_CONNECTOR_TOKEN` | Store-scoped connector token |
| `AST_TARGET_HOST` | Private database hostname |
| `AST_TARGET_PORT` | Private database port |
| `AST_SERVER` | Override server address (default: `fleet.astropods.ai:9092`) |

**Docker:**
```
docker run -d \
  -e AST_CONNECTOR_TOKEN=eyJ... \
  -e AST_TARGET_HOST=db.internal.example.com \
  -e AST_TARGET_PORT=5432 \
  astropods/ast-connect:latest
```

**Kubernetes:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ast-connect-postgres-prod
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: ast-connect
          image: astropods/ast-connect:latest
          env:
            - name: AST_CONNECTOR_TOKEN
              valueFrom:
                secretKeyRef:
                  name: astro-connector-token
                  key: token
            - name: AST_TARGET_HOST
              value: "db.internal.example.com"
            - name: AST_TARGET_PORT
              value: "5432"
```

**ECS:**
```json
{
  "family": "ast-connect-postgres-prod",
  "containerDefinitions": [{
    "name": "ast-connect",
    "image": "astropods/ast-connect:latest",
    "environment": [
      { "name": "AST_TARGET_HOST", "value": "db.internal.example.com" },
      { "name": "AST_TARGET_PORT", "value": "5432" }
    ],
    "secrets": [{
      "name": "AST_CONNECTOR_TOKEN",
      "valueFrom": "arn:aws:secretsmanager:us-east-1:123:secret:ast-connector-token"
    }]
  }]
}
```

### Redundancy

Multiple `ast-connect` instances connect simultaneously under the same store ARN. The server maintains a connector pool per ARN and round-robins `OpenStream` requests across healthy connectors. If a connector drops, in-flight streams receive a `StreamClose` and the agent's database client sees a TCP reset — standard reconnect behavior for database client libraries. Two replicas gives active-active redundancy with no failover delay.

### Security Isolation

- A connector token for `arn:knowledge:acme:postgres-prod` cannot receive streams for any other ARN — enforced server-side by the interceptor
- The connector opens no inbound ports — only outbound: one QUIC connection to the fleet endpoint and TCP connections to `target_host:target_port`
- Stream IDs are server-assigned UUIDs — agent pods cannot guess or interfere with each other's streams

---

## Performance Considerations for the QUIC Connector

### Why QUIC fits this use case

**Head-of-line blocking elimination.** In TCP, a single lost packet stalls all data on that connection until the packet is retransmitted. QUIC's streams are independent — a slow or stalled database query does not block a fast one running concurrently over the same tunnel. For agents running parallel vector searches or concurrent read/write operations, this is significant.

**0-RTT reconnection.** QUIC sessions can resume with 0-RTT when reconnecting to a known server. If the connector pod restarts or the network briefly drops, the tunnel re-establishes without a full TLS handshake. In practice this means a connector restart is invisible to active database connections that complete within the reconnect window (~100ms), rather than causing a hard TCP reset.

**Connection migration.** QUIC connections are identified by a connection ID, not the 4-tuple (src IP, src port, dst IP, dst port). If the connector pod is rescheduled onto a different node (changing its IP), the QUIC connection migrates without dropping. TCP would require a full reconnect and all in-flight streams would fail.

**Built-in TLS 1.3.** QUIC mandates encryption. There is no separate TLS handshake step — security is part of the connection setup. The existing fleet NLB uses QUIC-LB Connection ID routing, so TLS termination happens at the server pod, not the load balancer.

### Latency budget

A database query from an agent over the connector has the following hops compared to a direct connection:

```
Direct:    Agent → DB                                   (baseline)
Connector: Agent → astro-server → QUIC tunnel → ast-connect → DB
```

Each additional hop adds round-trip latency. Approximate breakdown for a typical cloud deployment (same region):

| Segment | Latency |
|---|---|
| Agent → astro-server (in-cluster) | < 1ms |
| astro-server → connector (QUIC, same region) | 1–5ms |
| connector → DB (in user VPC) | < 1ms |
| **Total overhead vs direct** | **~3–8ms per round trip** |

For LLM agent workloads — vector search, document retrieval, sparse reads — queries typically take 5–50ms. An overhead of 3–8ms is acceptable. For bulk ingestion jobs writing millions of rows in tight loops, the overhead compounds per round trip and PrivateLink is the better choice.

### Throughput ceiling

Each `StreamData` message carries a byte payload over the gRPC stream. The practical throughput ceiling is determined by:

1. **QUIC flow control** — QUIC has per-stream and per-connection flow control windows. Default `quic-go` settings allow up to 512KB per stream and 1MB per connection. For sustained high-throughput (e.g. bulk reads), these windows may need tuning.

2. **astro-server memory pressure** — every active stream buffers data in both directions in astro-server's process. At 1000 concurrent streams with 64KB buffers each, that is ~128MB of buffer memory in astro-server. The TunnelService should be given its own goroutine pool with bounded concurrency.

3. **Connector process limits** — `ast-connect` opens one TCP connection per stream. File descriptor limits (default 1024 on Linux) cap concurrent connections. Connectors should set `ulimit -n 65536` or the equivalent for their runtime.

For reference: `ast connect` (the CLI device connection) carries shell command stdout/stderr — low-bandwidth control traffic. The TunnelService carries raw database wire protocol — higher bandwidth, lower latency requirements. The two should be isolated in astro-server (separate goroutine pools, separate flow control budgets) to prevent a heavy knowledge store transfer from starving CLI device commands.

### UDP and firewall considerations

QUIC runs over UDP. Some corporate firewalls block or rate-limit UDP traffic by default. Before deploying `ast-connect`, users should verify:

- Outbound UDP to `fleet.astropods.ai:9092` is permitted
- No stateful UDP firewall rule with an aggressive idle timeout (QUIC keepalives fire every 30s by default — a firewall that closes UDP sessions after <30s of inactivity will break the tunnel)

If UDP is blocked, `ast-connect` should fall back to gRPC-over-TCP (port 443) automatically. This loses the QUIC benefits but maintains connectivity. The fallback path should be documented and tested.

### Connector sizing guide

| Concurrent agent connections to the store | Recommended connector replicas | Memory per connector |
|---|---|---|
| < 20 | 1 | 64MB |
| 20–100 | 2 | 128MB |
| 100–500 | 2–3 | 256MB |
| 500+ | Consider PrivateLink | — |

These are estimates based on 64KB stream buffers and typical database query sizes. Actual resource use depends heavily on query result size (a vector search returning 1000 embeddings is much larger than a simple row lookup).

---

## Cross-Cloud Connectivity

A common real-world scenario: Astro runs on AWS but the user's database lives on GCP, Azure, or a different AWS account or region. PrivateLink and the connector agent have fundamentally different answers to this.

### PrivateLink is intra-cloud only

PrivateLink operates entirely within a single cloud provider's backbone. AWS PrivateLink cannot reach a GCP database. GCP Private Service Connect cannot reach an Azure database. There is no cross-provider equivalent — the primitives simply do not cross cloud boundaries.

Within a single provider, cross-region PrivateLink is technically possible (AWS supports inter-region PrivateLink endpoints) but adds meaningful latency and cost, and requires the endpoint service and consumer to be in different AWS regions with explicit inter-region configuration. It is not the natural fit.

### The connector agent is cloud-agnostic by design

`ast-connect` only needs outbound internet access to `fleet.astropods.ai:9092`. It has no opinion about which cloud it runs on. The tunnel between the connector and Astro's fleet endpoint traverses the public internet, but is encrypted end-to-end by QUIC's mandatory TLS 1.3 — no data is exposed in transit.

Cross-cloud scenarios the connector handles natively:

| Astro runs on | Database lives on | Connector works |
|---|---|---|
| AWS | GCP | Yes |
| AWS | Azure | Yes |
| AWS | Different AWS account | Yes |
| AWS | Different AWS region | Yes |
| AWS | On-premises datacenter | Yes |
| GCP | AWS | Yes |
| Any | Any | Yes |

The network path in a cross-cloud deployment:

```
Agent Pod (AWS)
  └─→ astro-server (AWS, in-cluster TCP)
         └─→ fleet.astropods.ai:9092 (QUIC, internet)
                └─→ ast-connect (GCP / Azure / on-prem)
                       └─→ Private DB
```

The internet leg is encrypted by QUIC/TLS 1.3. The connector authenticates with its store-scoped token. No plaintext data traverses any network boundary.

### Cross-cloud without public internet

For regulated environments (financial services, healthcare, government) where compliance requirements prohibit database traffic traversing the public internet even when encrypted, the connector agent over the public internet is not acceptable. Options in that case:

**Cloud interconnect fabrics** — AWS Direct Connect, GCP Cloud Interconnect, and Azure ExpressRoute are physical private circuits between cloud providers and colocation facilities. If both Astro's AWS environment and the user's GCP environment are connected to the same colocation (e.g. Equinix), traffic can flow privately without touching the public internet. The connector agent works over this path — it just dials `fleet.astropods.ai:9092` over the private circuit rather than the internet.

**Third-party multi-cloud networking** — products like Aviatrix, Megaport, and Alkira create a unified private network fabric across AWS, GCP, and Azure. If a user already operates such a fabric, the connector agent traverses it natively. Alternatively, PrivateLink-style connectivity becomes possible if the fabric exposes endpoints on each cloud side.

**Dedicated Astro deployment (BYOC)** — running Astro's control plane inside the user's own cloud account eliminates the cross-cloud boundary entirely. This is a separate deployment model and out of scope here.

For most users, the connector agent over encrypted internet is sufficient. The compliance exception is narrow and well-understood — enterprises in regulated industries will already have interconnect infrastructure in place.

### Cross-region within the same cloud

If Astro and the database are on the same cloud provider but different regions, both options work but with different tradeoffs:

- **PrivateLink (inter-region)**: supported on AWS, but requires explicit inter-region endpoint configuration and adds ~$0.01/GB inter-region data transfer on top of standard PrivateLink costs. Latency reflects the geographic distance between regions.
- **Connector agent**: the connector runs in the database's region and dials Astro's fleet endpoint over the internet. Simpler to configure; same latency characteristics as cross-cloud. Preferred for cross-region unless the user already has PrivateLink infrastructure in place.

## Comparison

| | PrivateLink | Connector Agent |
|---|---|---|
| **Cloud support** | Single cloud provider only | Any cloud, any region, on-premises |
| **Cross-cloud** | Not supported | Native |
| **Setup** | NLB + endpoint service + acceptance flow | Deploy one container |
| **Network latency** | Near-native (one NLB hop) | Extra hop through astro-server (~3–8ms) |
| **Throughput** | Line-rate native TCP | Bounded by QUIC flow control and astro-server buffers |
| **Works on-premises** | No | Yes |
| **UDP requirement** | No | Yes (with TCP fallback) |
| **Public internet traversal** | No | Yes (encrypted TLS 1.3) |
| **Redundancy** | NLB multi-AZ by default | Multiple connector replicas, server-side pool |
| **Cost** | Cloud provider endpoint fees | Included in platform (reuses fleet infra) |
| **Best for** | High-throughput, single-cloud, regulated same-cloud environments | Multi-cloud, cross-cloud, on-prem, quick setup |

---

## Store Status Model

| Status | Meaning |
|---|---|
| `pending` | Created, no connectivity configured |
| `connecting` | Connector not yet online / endpoint creation in progress |
| `pending-acceptance` | PrivateLink endpoint awaiting user approval |
| `ready` | Connectivity established, store reachable |
| `degraded` | Connector disconnected / endpoint unhealthy but recoverable |
| `error` | Permanently failed (token revoked, endpoint rejected) |

Agents bound to a store in `degraded` or `error` will fail at the DB connection level. The platform surfaces this in the deployment health view.
