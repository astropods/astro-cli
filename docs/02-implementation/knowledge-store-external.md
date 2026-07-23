# Knowledge Store — External Implementation

Scope: external (bring-your-own) knowledge stores and PrivateLink automation. Managed stores and the connector agent are out of scope here.

---

## What Changes

Today, all knowledge stores are managed — the platform provisions and operates the database. Users with existing databases cannot onboard them. The only PrivateLink in the system is Langfuse, which is a static Terraform resource.

After this change, a user can onboard an existing database under an ARN. The platform stores encrypted credentials and injects them into bound agents at deploy time. No infrastructure is created for the database itself. For private databases unreachable from Astro's network, PrivateLink is automated at runtime via the AWS SDK — no Terraform apply per store.

Managed stores continue to work unchanged. External stores share the same `knowledge_stores` table, ARN scheme, binding mechanism, and credential encryption. The difference is mode: managed stores have K8s resources; external stores have only credentials (and optionally a VPC endpoint).

---

## Phase 1 — External Store Lifecycle

Onboard an existing database, store encrypted credentials, and expose it under an ARN.

### Data Model

#### Schema changes to `knowledge_stores`

Add a `mode` column to distinguish managed from external:

```sql
ALTER TABLE knowledge_stores
  ADD COLUMN mode varchar NOT NULL DEFAULT 'managed';
```

For external stores, the `storage` column is irrelevant (defaults to `'10Gi'`, ignored). The `status` column gains new values for the PrivateLink flow (Phase 2). The `public` and `public_host` columns are unused for external stores.

Status lifecycle for external stores: `pending` → `ready` | `error`

No provisioning state — there is nothing to provision. A publicly reachable external store goes directly to `ready` after credential storage succeeds.

#### New status constants

```go
const (
    StatusPending          = "pending"            // external: created, connectivity not yet verified
    StatusPendingAcceptance = "pending-acceptance" // PrivateLink: VPCE awaiting user approval
    StatusConnecting       = "connecting"          // PrivateLink: VPCE creation in progress
    StatusDegraded         = "degraded"            // connectivity lost but recoverable
)
```

These are added alongside the existing `provisioning`, `ready`, `error`.

### Credential Storage

External stores reuse the existing `knowledge_store_credentials` table and KMS envelope encryption. The only difference is the source of the credential values.

For managed stores, `GenerateCredentials()` in `credentials.go` creates random secrets. For external stores, the values come from the user — host, port, database, username, password, and any provider-specific extras.

External credential keys per provider:

| Provider | Keys |
|----------|------|
| `postgres` | `HOST`, `PORT`, `DATABASE`, `USERNAME`, `PASSWORD` |
| `qdrant` | `HOST`, `PORT`, `API_KEY` |
| `redis` | `HOST`, `PORT`, `PASSWORD` |
| `neo4j` | `HOST`, `PORT`, `USERNAME`, `PASSWORD` |
| `pinecone` | `HOST`, `API_KEY` |
| `mysql` | `HOST`, `PORT`, `DATABASE`, `USERNAME`, `PASSWORD` |

The key names are unprefixed. The env var prefix is determined at inject time by the binding entry name and provider, matching the existing convention in `envresolver.go`.

No K8s Secret is created for external stores. There is no StatefulSet to consume it. Credentials are decrypted from the DB at deploy time and injected directly into the agent pod spec as env vars (or as a K8s Secret in the agent's namespace, depending on the binding implementation in Phase 2 of the managed doc).

### API

New endpoint for onboarding external stores, alongside the existing create endpoint for managed stores.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/accounts/{account}/knowledge/connect` | Onboard external store |

Existing endpoints work unchanged for external stores: list, get, delete, credentials.

**POST `/connect` body:**

```json
{
  "name": "postgres-prod",
  "provider": "postgres",
  "host": "db.example.com",
  "port": 5432,
  "database": "vectors",
  "username": "app",
  "password": "secret"
}
```

**Response** (same shape as managed):

```json
{
  "arn": "arn:knowledge:acme:postgres-prod",
  "provider": "postgres",
  "status": "ready",
  "mode": "external"
}
```

The response includes the new `mode` field. Existing list/get responses also gain this field — `"managed"` for existing stores.

**Handler implementation** (`handlers/knowledge.go`):

1. Validate name (existing `ValidateStoreName`), provider (existing `spec.LookupBuiltin`), and required fields per provider.
2. Generate store ID (nanoid) and ARN.
3. Generate KMS data key (existing `envelope.GenerateDataKey`).
4. Encrypt user-provided credentials (existing `EncryptCredentials` path).
5. Insert `knowledge_stores` row with `mode='external'`, `status='ready'`.
6. Insert `knowledge_store_credentials` rows.
7. Return 200 with ARN.

No async provisioning, no background goroutine, no K8s resources. The handler is synchronous.

### CLI

New subcommand under the existing `ast knowledge` group:

```
ast knowledge connect \
  --name <name> \
  --provider <provider> \
  --host <host> \
  --port <port> \
  [--database <db>] \
  [--username <user>] \
  [--password <pass>]
```

If `--password` is omitted, the CLI prompts interactively (same pattern as existing credential prompts). The CLI posts to `/connect` and prints the ARN on success. No event streaming — the operation is synchronous.

The existing `ast knowledge list` shows both managed and external stores. The `mode` column is displayed. `ast knowledge credentials` works for external stores — decrypts from DB as it does for managed.

`ast knowledge delete` for external stores skips K8s cleanup (no resources to delete) and removes only the DB rows and any PrivateLink resources (Phase 2).

### Connect Flow

```
ast knowledge connect --name postgres-prod --provider postgres --host db.example.com ...
  |
  v
POST /v1/accounts/acme/knowledge/connect
  |
  +- Validate name, provider, required fields
  +- Generate store ID + ARN
  +- Generate KMS data key
  +- Encrypt credentials (host, port, database, username, password)
  +- INSERT knowledge_stores (mode=external, status=ready)
  +- INSERT knowledge_store_credentials
  +- Return 200 { arn, status: "ready", mode: "external" }
```

### Binding

External stores use the same binding mechanism as managed stores (Phase 2 of the managed doc). The difference is in env var resolution at deploy time:

- **Managed**: construct service DNS `{id}.knowledge-{accountShortID}.svc.cluster.local` for the host.
- **External**: decrypt `HOST` and `PORT` from the DB and inject directly.

The `template.go` resolution path branches on `mode`:

```
if store.Mode == "managed":
    host = "{store.ID}.knowledge-{accountShortID}.svc.cluster.local"
    port = provider default port
else:
    host = decrypt(store credentials, "HOST")
    port = decrypt(store credentials, "PORT")
```

Provider mismatch validation is identical — the spec entry declares a provider, the store has a provider, mismatch is a hard error.

---

## Phase 2 — PrivateLink Automation

Automate VPC endpoint creation for external stores on private networks. AWS first; GCP and Azure follow the same pattern with different API calls.

### Why not Terraform

The Langfuse PrivateLink in `terraform/environments/prod/privatelink-langfuse.tf` is static — one endpoint, never changes, hardcoded NLB and VPCE. Knowledge store PrivateLink is dynamic: user-triggered, N endpoints per account, created and deleted at any time. Terraform would require a plan/apply cycle per onboard. This must be a runtime controller.

### Infrastructure Envelope (Terraform)

Terraform owns the static envelope — resources that exist once and never change per environment:

**1. Security group** — one per managed cluster VPC, empty on creation:

```hcl
resource "aws_security_group" "knowledge_privatelink" {
  name_prefix = "${var.environment}-knowledge-pl-"
  description = "Egress rules for knowledge store PrivateLink endpoints"
  vpc_id      = module.vpc_managed.vpc_id

  tags = merge(local.common_tags, {
    Name = "${var.environment}-knowledge-privatelink"
  })
}
```

No ingress or egress rules. Rules are added/removed at runtime by astro-server per endpoint.

**2. IRSA policy** — extends the existing astro-server service account role:

```json
{
  "Effect": "Allow",
  "Action": [
    "ec2:CreateVpcEndpoint",
    "ec2:DeleteVpcEndpoints",
    "ec2:DescribeVpcEndpoints",
    "ec2:DescribeNetworkInterfaces",
    "ec2:ModifyVpcEndpoint",
    "ec2:AuthorizeSecurityGroupEgress",
    "ec2:RevokeSecurityGroupEgress",
    "ec2:CreateTags"
  ],
  "Resource": "*"
}
```

Scoping by tag condition (`astro.io/component: knowledge`) where supported by the API action.

**3. Config injection** — new env vars in the Helm values for astro-server:

| Env var | Source | Description |
|---------|--------|-------------|
| `PRIVATELINK_VPC_ID` | `module.vpc_managed.vpc_id` | Managed cluster VPC |
| `PRIVATELINK_SUBNET_IDS` | `module.vpc_managed.private_subnet_ids` | Comma-separated |
| `PRIVATELINK_SG_ID` | `aws_security_group.knowledge_privatelink.id` | Empty SG shell |

These follow the existing pattern of injecting infra outputs as env vars (cf. `POD_SUBNET_CIDRS`, `LANGFUSE_VPCE_IPS`).

### Data Model

#### New table: `knowledge_store_endpoints`

```sql
CREATE TABLE public.knowledge_store_endpoints (
    knowledge_store_id varchar(11) NOT NULL REFERENCES public.knowledge_stores(id) ON DELETE CASCADE,
    cloud_provider     varchar     NOT NULL,  -- 'aws', 'gcp', 'azure'
    endpoint_service   varchar     NOT NULL,  -- user's service name / attachment URI / resource ID
    region             varchar     NOT NULL,
    endpoint_id        varchar,               -- VPCE ID once created (e.g. vpce-0abc123)
    endpoint_dns       varchar,               -- resolved DNS name once available
    status             varchar     NOT NULL DEFAULT 'connecting',
    error              text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (knowledge_store_id)
);

CREATE INDEX idx_knowledge_store_endpoints_status ON knowledge_store_endpoints(status);
```

One endpoint per store (1:1). A store that needs PrivateLink has exactly one endpoint row. Stores without PrivateLink have no row.

### Config

Add to `DeploymentConfig` in `config.go`:

```go
// PrivateLink automation — managed cluster VPC where endpoints are created
PrivateLinkVpcID     string   // PRIVATELINK_VPC_ID
PrivateLinkSubnetIDs []string // PRIVATELINK_SUBNET_IDS (comma-separated)
PrivateLinkSGID      string   // PRIVATELINK_SG_ID
```

PrivateLink is disabled when `PRIVATELINK_VPC_ID` is empty (local dev, environments without a managed cluster).

### API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/accounts/{account}/knowledge/{name}/privatelink` | Attach PrivateLink |
| `DELETE` | `/v1/accounts/{account}/knowledge/{name}/privatelink` | Detach PrivateLink |

**POST body:**

```json
{
  "cloud_provider": "aws",
  "service": "com.amazonaws.vpce.us-east-1.vpce-svc-0abc123",
  "region": "us-east-1"
}
```

**Handler:**

1. Verify the store exists, is external, and has no existing endpoint.
2. Verify PrivateLink config is present (`PRIVATELINK_VPC_ID` non-empty).
3. Insert `knowledge_store_endpoints` row with `status='connecting'`.
4. Update `knowledge_stores.status` to `connecting`.
5. Enqueue a `PrivateLinkProvisionArgs` River job.
6. Return 202.

The actual AWS API call is not made in the handler — it is made by the River worker. This keeps the handler fast and lets the worker handle retries.

**DELETE handler:**

1. Look up the endpoint row.
2. Enqueue a `PrivateLinkDeleteArgs` River job.
3. Return 202.

### CLI

```
ast knowledge privatelink attach \
  --store <name-or-arn> \
  --provider aws \
  --service com.amazonaws.vpce.us-east-1.vpce-svc-0abc123 \
  --region us-east-1

ast knowledge privatelink detach --store <name-or-arn>
```

After `attach`, the CLI polls `GET /v1/accounts/{account}/knowledge/{name}` until status changes from `connecting` to `pending-acceptance`, `ready`, or `error`. Same 3-second poll pattern as `ast knowledge create`.

### PrivateLink Provision Worker

New River worker: `PrivateLinkProvisionWorker`.

```go
type PrivateLinkProvisionArgs struct {
    StoreID string `json:"store_id"`
}

func (PrivateLinkProvisionArgs) Kind() string { return "privatelink.provision" }
```

This is a one-shot job (not periodic). Enqueued by the handler.

**Work function:**

1. Load store and endpoint row from DB.
2. Create EC2 client via `aws-sdk-go-v2` (same `LoadDefaultConfig` pattern as `loadKMSClient` in `knowledge_reconcile.go`).
3. Call `ec2.CreateVpcEndpoint`:
   - `VpcEndpointType`: `Interface`
   - `VpcId`: from config
   - `ServiceName`: from endpoint row
   - `SubnetIds`: from config
   - `SecurityGroupIds`: from config
   - `TagSpecifications`: `astro.io/store-id`, `astro.io/account-id`, `astro.io/component: knowledge`
4. Record the returned VPCE ID in the endpoint row.
5. Update endpoint status to `pending-acceptance`.
6. Update store status to `pending-acceptance`.

If `CreateVpcEndpoint` fails (invalid service name, account not allowlisted), record the error and set both statuses to `error`.

### Reconciliation

The existing `KnowledgeReconcileWorker` (30-second periodic job) gains a third stage: `reconcilePrivateLink`.

```
func (w *KnowledgeReconcileWorker) Work(ctx, job):
    w.reconcileProvisioning(ctx)    // existing — managed stores
    w.ensureSecrets(ctx)            // existing — managed credential recovery
    w.reconcilePrivateLink(ctx)     // new — external PrivateLink endpoints
```

**`reconcilePrivateLink` logic:**

```
for each endpoint where status in ('pending-acceptance', 'connecting'):
    ec2.DescribeVpcEndpoints(endpoint.EndpointID)

    switch vpce.State:
    case "pendingAcceptance":
        update endpoint status to 'pending-acceptance' (if not already)

    case "available":
        dns = vpce.DnsEntries[0].DnsName
        update endpoint: status='ready', endpoint_dns=dns
        update store: status='ready'
        add SG egress rule (see below)
        create NetworkPolicy (see below)

    case "rejected", "failed", "deleted":
        update endpoint: status='error', error=reason
        update store: status='error', error=reason
```

### Security Group Rules

When an endpoint becomes `available`, the worker adds an egress rule to the shared PrivateLink SG:

```go
ec2Client.AuthorizeSecurityGroupEgress(ctx, &ec2.AuthorizeSecurityGroupEgressInput{
    GroupId: cfg.PrivateLinkSGID,
    IpPermissions: []types.IpPermission{{
        IpProtocol: aws.String("tcp"),
        FromPort:   aws.Int32(port),
        ToPort:     aws.Int32(port),
        UserIdGroupPairs: []types.UserIdGroupPair{{
            // Target the VPCE's ENI security group — not CIDR-based
        }},
    }},
    // Or use PrefixListIds if ENI IPs are not stable
})
```

On delete, the corresponding rule is revoked via `RevokeSecurityGroupEgress`. Idempotent — revoke of a non-existent rule is a no-op.

### NetworkPolicy

Agent pods need egress to the VPCE ENI IPs. When an endpoint reaches `ready`:

1. Resolve VPCE ENI private IPs via `ec2.DescribeNetworkInterfaces` (same pattern as the Langfuse PrivateLink in `privatelink-langfuse.tf` which uses `data.aws_network_interface`).
2. For each namespace with an active binding to this store, create a `NetworkPolicy`:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kn-{storeID}-egress
  namespace: {agent-namespace}
  labels:
    astro.io/store-id: {storeID}
    astro.io/component: knowledge-privatelink
spec:
  podSelector: {}
  policyTypes: ["Egress"]
  egress:
    - to:
        - ipBlock:
            cidr: {eni-ip-1}/32
        - ipBlock:
            cidr: {eni-ip-2}/32
      ports:
        - port: {db-port}
          protocol: TCP
```

For Phase 2, this is created reactively when the endpoint becomes available and when new bindings are created for a PrivateLink-backed store. The existing CronJob pattern (Langfuse NetworkPolicy sync every 12 hours) serves as a fallback reconciliation — it can be extended to include knowledge PrivateLink endpoints in its sweep.

On store delete or PrivateLink detach, the NetworkPolicy is removed from all agent namespaces.

### PrivateLink Delete Worker

New River worker: `PrivateLinkDeleteWorker`.

```go
type PrivateLinkDeleteArgs struct {
    StoreID string `json:"store_id"`
}

func (PrivateLinkDeleteArgs) Kind() string { return "privatelink.delete" }
```

**Work function:**

1. Load endpoint row.
2. If `endpoint_id` is set, call `ec2.DeleteVpcEndpoints`.
3. Revoke the SG egress rule (idempotent).
4. Delete NetworkPolicy objects labeled `astro.io/store-id={storeID}` across all namespaces.
5. Delete the endpoint row.
6. If the store is also being deleted (triggered by `ast knowledge delete`), proceed with store row deletion. Otherwise, revert store status to `pending` (PrivateLink removed but store still exists).

All steps are idempotent — re-running after partial failure is safe.

### Store Delete Flow (External with PrivateLink)

```
ast knowledge delete postgres-prod
  |
  v
DELETE /v1/accounts/acme/knowledge/postgres-prod
  |
  +- Check active bindings -> 409 if any
  +- If endpoint row exists: enqueue PrivateLinkDeleteArgs
  +- Delete knowledge_store_credentials rows
  +- Delete knowledge_stores row (cascades endpoint row via ON DELETE CASCADE)
  +- Return 200
```

The PrivateLink delete job runs even though the DB rows are already gone — it operates on the AWS resources (VPCE, SG rules) and K8s resources (NetworkPolicies) using the IDs it captured from the endpoint row before deletion. The job args carry the VPCE ID directly to handle this.

### Acceptance Flow (User Experience)

```
$ ast knowledge privatelink attach \
    --store postgres-prod \
    --provider aws \
    --service com.amazonaws.vpce.us-east-1.vpce-svc-0abc123 \
    --region us-east-1

Attaching PrivateLink to arn:knowledge:acme:postgres-prod...
Status: connecting
Status: pending-acceptance

  Action required: accept the endpoint connection request in your AWS console.
  Endpoint service: com.amazonaws.vpce.us-east-1.vpce-svc-0abc123
  Astro VPC endpoint: vpce-0def456...

  Waiting for acceptance...

Status: ready
Endpoint: vpce-0def456-x.us-east-1.vpce.amazonaws.com
```

The CLI detects `pending-acceptance` and prints the action-required message. It continues polling until `ready` or `error`, or the user interrupts with Ctrl-C (the endpoint stays in `pending-acceptance` and will be picked up by the reconciler when eventually accepted).

If the user's endpoint service is configured with auto-accept for Astro's account ID, the status transitions directly from `connecting` to `ready` with no manual step.

### Env Var Injection with PrivateLink

When a PrivateLink-backed store is bound to an agent, the `HOST` injected into the agent is the VPCE DNS name — not the user's original host:

```
POSTGRES_HOST = vpce-0def456-x.us-east-1.vpce.amazonaws.com
POSTGRES_PORT = 5432
```

The original `HOST` credential in the DB (`db.example.com`) is the user's private host — unreachable from Astro's VPC. The VPCE DNS resolves to ENI private IPs within Astro's VPC that route to the user's NLB via the cloud backbone.

The `template.go` resolution path for external + PrivateLink:

```
if store.Mode == "external":
    endpoint = lookup knowledge_store_endpoints for store
    if endpoint exists and endpoint.Status == "ready":
        host = endpoint.EndpointDNS
    else:
        host = decrypt(store credentials, "HOST")  // public external, or PrivateLink not yet ready
    port = decrypt(store credentials, "PORT")
```

---

## IRSA Policy Scoping

The EC2 permissions for PrivateLink should be split into two statements:

1. **Describe/create** (broad — AWS does not support resource-level constraints for these):
   - `ec2:CreateVpcEndpoint`, `ec2:DescribeVpcEndpoints`, `ec2:DescribeNetworkInterfaces`, `ec2:DeleteVpcEndpoints`, `ec2:ModifyVpcEndpoint`, `ec2:CreateTags`
   - `Resource: "*"` (unavoidable)

2. **Security group mutations** (scoped to the PrivateLink SG ARN):
   - `ec2:AuthorizeSecurityGroupEgress`, `ec2:RevokeSecurityGroupEgress`
   - `Resource: arn:aws:ec2:{region}:{account}:security-group/{sg-id}`

This limits blast radius — even if the server is compromised, SG mutations are constrained to the single PrivateLink SG.

---

## Known Limitations

**Security group rule limits.** The shared PrivateLink SG accumulates egress rules as stores are added. AWS SGs have a default limit of 60 rules (inbound + outbound combined). With many external stores, this will be hit. Mitigations: request a limit increase, or switch to a prefix list strategy.

**VPC endpoint limits.** AWS default is 50 interface VPC endpoints per VPC per region. Request a limit increase proactively if expecting more than ~40 external PrivateLink stores per environment.

**No connection health checks.** A store can be `status=ready` but actually unreachable (wrong credentials, firewall changed, endpoint service deregistered). Users discover this at agent deploy time or when queries fail. Health checks are out of scope for this phase.

**NetworkPolicy reconciliation gap.** PrivateLink NetworkPolicies are created by the River worker at runtime, not by the CronJob. If the worker crashes mid-create, the NetworkPolicy may be missing until the next reconcile cycle picks it up. The CronJob can be extended as a fallback but is not specced here.

**DNS propagation delay.** When a VPCE transitions to `available`, DNS entries may take a few seconds to propagate. The reconciler handles this by deferring to the next cycle if `DnsEntries` is empty on the first `available` poll.

---

## What Is Not Built Here

- Connector agent (QUIC tunnel for cross-cloud / on-premises)
- GCP Private Service Connect automation (same pattern, different API)
- Azure Private Link automation (same pattern, different API)
- Connection health checks (periodic connectivity verification)
- PrivateLink cost tracking and passthrough billing
- NetworkPolicy CronJob fallback reconciliation
- UI (Knowledge section) — API-first; UI follows separately
