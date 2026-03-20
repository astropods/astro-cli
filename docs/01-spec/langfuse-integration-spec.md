# Langfuse Integration Spec

Add Langfuse as a second observability backend alongside Galileo. Both run in parallel at the collector level — same traces fan out to both. Each Astro account gets its own Langfuse project, provisioned by writing directly to Langfuse's Postgres database.

Langfuse instances: `https://langfuse.astropod.ai/` and `https://langfuse.astropods.ai/` (self-hosted, v3.22.0+).

## 1. Per-account Langfuse project provisioning

### Problem

Galileo uses a single shared project with log-stream-based isolation. Langfuse scopes traces to a project (determined by API keys) with no log stream concept. A shared project would mix traces across tenants. Each Astro account needs its own Langfuse project.

### Why direct DB provisioning

Langfuse's public API for creating projects and API keys (`POST /api/public/projects`, `POST /api/public/projects/{id}/apiKeys`) requires the `admin-api` entitlement, gated behind an enterprise license (`LANGFUSE_EE_LICENSE_KEY`). We don't have one. The internal tRPC endpoints use cookie-based session auth, which is fragile to scrape.

Since we self-host Langfuse, we have direct access to its Postgres database. The schema is stable (Prisma-managed) and the provisioning logic is straightforward.

### Langfuse database schema (reference)

Langfuse uses three tables for project/key management:

**`organizations` table** (already exists — one org for our Langfuse instance):

| Column | Type | Notes |
|--------|------|-------|
| `id` | text (cuid) | PK |
| `name` | text | |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |

**`projects` table** — one row per Astro account:

| Column | Type | Notes |
|--------|------|-------|
| `id` | text (cuid) | PK |
| `org_id` | text | FK → organizations.id |
| `name` | text | `astro-{account_name}` |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |

**`api_keys` table** — one row per project:

| Column | Type | Notes |
|--------|------|-------|
| `id` | text (cuid) | PK |
| `public_key` | text (unique) | Format: `pk-lf-{uuid}` |
| `hashed_secret_key` | text (unique) | bcrypt(sk, cost=11) |
| `fast_hashed_secret_key` | text (unique) | `sha256(sk + sha256(SALT))` |
| `display_secret_key` | text | `sk[:6]...sk[-4:]` |
| `project_id` | text | FK → projects.id |
| `scope` | enum | `PROJECT` |
| `created_at` | timestamp | |

### Key generation logic

Langfuse generates keys as:
- Public key: `pk-lf-{uuid}`
- Secret key: `sk-lf-{uuid}`

Storage requires three derived values from the secret key:
1. **`hashed_secret_key`** — `bcrypt(secret_key, cost=11)` (legacy auth path)
2. **`fast_hashed_secret_key`** — `sha256(secret_key + sha256(SALT))` where SALT is Langfuse's `SALT` env var (fast auth path)
3. **`display_secret_key`** — `secret_key[:6] + "..." + secret_key[-4:]`

The `SALT` value must be read from the Langfuse deployment's environment. It's a required env var for Langfuse — we need to configure astro-server with the same value.

### Astro-side storage

New table in astro-server's database to cache per-account Langfuse credentials:

```sql
CREATE TABLE public.account_langfuse (
    account_id uuid NOT NULL,
    langfuse_project_id text NOT NULL,
    langfuse_public_key text NOT NULL,
    langfuse_secret_key text NOT NULL,   -- KMS-encrypted (same pattern as deployment_variables)
    nonce bytea,
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT account_langfuse_pkey PRIMARY KEY (account_id),
    CONSTRAINT account_langfuse_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts(id) ON DELETE CASCADE
);
```

We store the raw secret key (KMS-encrypted) so we can use it for both OTLP auth (collector) and REST API queries (proxy handlers). The hashed versions only live in Langfuse's DB.

### Provisioning flow

Lazy provisioning on first deployment for an account:

```
deployer.Apply(dep) for account "acme"
  → check account_langfuse for account_id
  → if not found:
      1. Generate keys: pk = "pk-lf-{uuid}", sk = "sk-lf-{uuid}"
      2. Generate cuid for project ID and API key ID
      3. Compute hashes:
         - bcrypt(sk, cost=11)
         - sha256(sk + sha256(LANGFUSE_SALT))
         - display: sk[:6] + "..." + sk[-4:]
      4. INSERT INTO langfuse_db.projects (id, org_id, name="astro-acme", ...)
      5. INSERT INTO langfuse_db.api_keys (id, public_key, hashed_secret_key, fast_hashed_secret_key, display_secret_key, project_id, scope="PROJECT")
      6. KMS-encrypt sk
      7. INSERT INTO astro_db.account_langfuse (account_id, project_id, public_key, encrypted_secret_key, nonce)
  → compute auth token: base64(pk + ":" + sk)
  → inject LANGFUSE_AUTH_TOKEN into collector container
```

### Provisioner (`apps/astro-server/internal/langfuse/provisioner.go`)

```go
type Provisioner struct {
    langfuseDB *sql.DB   // direct connection to Langfuse's Postgres
    salt       string     // Langfuse's SALT env var value
    orgID      string     // Langfuse org ID (single org for our instance)
}

// EnsureProject creates a Langfuse project + API key for the account if not yet provisioned.
// Returns public key and secret key.
func (p *Provisioner) EnsureProject(ctx context.Context, store *Store, accountID, accountName string) (pk, sk string, err error)
```

The provisioner needs:
- A Postgres connection to Langfuse's database (separate from astro-server's DB)
- The `SALT` value from Langfuse's environment
- The org ID of the Langfuse organization (queried once at startup or configured via env var)

### Server config

Add to `DeploymentConfig`:

```go
// Langfuse — direct DB provisioning
LangfuseDBURL  string // LANGFUSE_DB_URL — Postgres connection string for Langfuse's database
LangfuseSalt   string // LANGFUSE_SALT — must match Langfuse's SALT env var
LangfuseOrgID  string // LANGFUSE_ORG_ID — the single org ID in our Langfuse instance
LangfuseBaseURL string // LANGFUSE_BASE_URL (default: https://langfuse.astropods.ai)
```

### Naming convention

Langfuse project name: `astro-{account_name}` (e.g., `astro-acme`). Unique because account names are unique.

## 2. Collector — dual export

### Config change (`packages/astro-collector/config/collector-config.yaml`)

Add a second OTLP/HTTP exporter targeting Langfuse:

```yaml
exporters:
  otlp_http/galileo:
    # ... (unchanged)

  otlp_http/langfuse:
    traces_endpoint: ${env:LANGFUSE_OTLP_ENDPOINT:-https://langfuse.astropods.ai/api/public/otel/v1/traces}
    metrics_endpoint: ${env:LANGFUSE_OTLP_ENDPOINT:-https://langfuse.astropods.ai/api/public/otel/v1/metrics}
    logs_endpoint: ${env:LANGFUSE_OTLP_ENDPOINT:-https://langfuse.astropods.ai/api/public/otel/v1/logs}
    headers:
      Authorization: Basic ${env:LANGFUSE_AUTH_TOKEN}
      x-langfuse-ingestion-version: "4"
    retry_on_failure:
      enabled: true
      max_elapsed_time: 30s
```

Wire into all three pipelines:

```yaml
service:
  pipelines:
    traces:
      exporters: [debug, otlp_http/galileo, otlp_http/langfuse]
    metrics:
      exporters: [otlp_http/galileo, otlp_http/langfuse]
    logs:
      exporters: [otlp_http/galileo, otlp_http/langfuse]
```

### Auth token

`LANGFUSE_AUTH_TOKEN` = `base64(pk:sk)` — computed per-deploy from the account's credentials in `account_langfuse`.

### Graceful degradation

If `LANGFUSE_AUTH_TOKEN` is empty, the Langfuse exporter fails auth silently. Galileo continues unaffected.

## 3. Deploy-time credential injection

### Flow through the deployer

```
deployer.Apply(dep)
  → acct := accountStore.GetByID(dep.AccountID)
  → langfuseCreds, err := langfuseStore.Get(acct.ID)
  → if langfuseCreds == nil && provisioner != nil:
      pk, sk := provisioner.EnsureProject(ctx, store, acct.ID, acct.Name)
      langfuseCreds = &AccountLangfuse{PublicKey: pk, SecretKey: sk}
  → authToken := base64(langfuseCreds.PublicKey + ":" + langfuseCreds.SecretKey)
  → applier := k8s.NewApplier(..., ApplierConfig{
      LangfuseAuthToken: authToken,
    })
```

### K8s deployment (`apps/astro-server/internal/k8s/deployment.go`)

Add to `CollectorDeploymentConfig`:

```go
LangfuseAuthToken string // per-account base64(pk:sk)
```

In `buildCollectorContainer()`:

```go
if cfg.LangfuseAuthToken != "" {
    container.Env = append(container.Env, corev1.EnvVar{
        Name: "LANGFUSE_AUTH_TOKEN", Value: cfg.LangfuseAuthToken,
    })
}
```

### `ApplierConfig` and `spec_applier.go`

Add `LangfuseAuthToken string` to `ApplierConfig`. Pass through to `CollectorDeploymentConfig` when building the collector sidecar.

## 4. Langfuse query client (`apps/astro-server/internal/langfuse/client.go`)

REST client for reading traces back. Uses per-account project keys from `account_langfuse`.

### Key endpoints

| Method | Langfuse API | Purpose |
|--------|-------------|---------|
| `GetTraces` | `GET /api/public/traces?tags=agent:{name}` | List traces filtered by agent |
| `GetTrace` | `GET /api/public/traces/{traceId}` | Single trace with observations |
| `GetMetrics` | `GET /api/public/metrics/daily` | Daily metrics |

### Trace scoping

Each account has its own Langfuse project, so all traces in the project belong to that account. To scope to a specific agent within the project, filter by:
- Tag `agent:{agentName}` (set by astro processor)
- Tag `deployment:{deploymentId}` for deployment-level filtering

### Client struct

```go
type Client struct {
    baseURL   string
    publicKey string
    secretKey string
    http      *http.Client
}

func NewClient(baseURL, publicKey, secretKey string) *Client
```

Auth: `Authorization: Basic base64(pk:sk)` on every request.

## 5. Server proxy handlers

### Endpoint design

Parallel endpoints under a `/langfuse/` prefix:

```
GET /api/v1/agents/:account/:name/observability/langfuse/traces
GET /api/v1/agents/:account/:name/observability/langfuse/metrics
GET /api/v1/agents/:account/:name/observability/langfuse/summary
```

### Handler implementation

```go
func resolveObservabilityLangfuseContext(
    c *gin.Context, log *logger.Logger, cfg *config.Config,
    accountStore *account.AccountStore,
    langfuseStore *langfuse.Store,
) (*langfuse.Client, bool) {
    // Auth + membership checks (same as Galileo)
    // ...
    creds, err := langfuseStore.Get(acct.ID)
    if err != nil || creds == nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": "langfuse not configured for this account"})
        return nil, false
    }
    client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)
    return client, true
}
```

Per-account credentials from the database, not server-wide env vars.

## 6. CLI local dev (`apps/astro-cli/internal/compose/builder.go`)

Forward Langfuse env vars if present in `buildCollectorEnvironment()`:

```go
for _, key := range []string{"LANGFUSE_AUTH_TOKEN", "LANGFUSE_OTLP_ENDPOINT"} {
    if v, ok := envVars[key]; ok {
        val := v
        env[key] = &val
    }
}
```

## 7. Files to modify/create

| File | Change |
|------|--------|
| `sql/astro-server/schema.sql` | Add `account_langfuse` table |
| `packages/astro-collector/config/collector-config.yaml` | Add `otlp_http/langfuse` exporter, wire into pipelines |
| `apps/astro-server/internal/config/config.go` | Add `LangfuseDBURL`, `LangfuseSalt`, `LangfuseOrgID`, `LangfuseBaseURL` |
| `apps/astro-server/internal/langfuse/provisioner.go` | **New** — direct DB provisioning (project + API key creation) |
| `apps/astro-server/internal/langfuse/store.go` | **New** — astro DB store for `account_langfuse` |
| `apps/astro-server/internal/langfuse/client.go` | **New** — Langfuse REST API client for trace queries |
| `apps/astro-server/internal/deployer/deployer.go` | Add Langfuse provisioning + credential lookup in `Apply()` |
| `apps/astro-server/internal/k8s/deployment.go` | Add `LangfuseAuthToken` to `CollectorDeploymentConfig`, inject env var |
| `apps/astro-server/internal/k8s/spec_applier.go` | Pass `LangfuseAuthToken` through to collector config |
| `apps/astro-server/handlers/observability.go` | Add Langfuse proxy endpoints using per-account credentials |
| `apps/astro-server/handlers/routes.go` | Register `/langfuse/` routes |
| `apps/astro-cli/internal/compose/builder.go` | Forward `LANGFUSE_AUTH_TOKEN` in local dev |

## 8. Implementation order

1. **Schema + store** — Add `account_langfuse` table to astro DB. Build `Store` with KMS encryption for secret keys.
2. **Provisioner** — Build the direct-DB provisioner: connect to Langfuse Postgres, generate keys, compute hashes, insert project + api_key rows.
3. **Collector config** — Add the Langfuse exporter to `collector-config.yaml`.
4. **Deploy-time wiring** — In `deployer.Apply()`, call `EnsureProject()`, compute auth token, pass through `ApplierConfig` → `CollectorDeploymentConfig` → collector env var. Traces start flowing.
5. **Query client + proxy endpoints** — Build Langfuse REST client and `/langfuse/` handlers.
6. **CLI local dev** — Forward env vars in compose builder.

After step 4, traces flow to both backends. Use the Langfuse UI at `langfuse.astropods.ai` while proxy endpoints are built.

## 9. Environment variables summary

| Variable | Where set | Purpose |
|----------|-----------|---------|
| `LANGFUSE_DB_URL` | Server env | Postgres connection string for Langfuse's database |
| `LANGFUSE_SALT` | Server env | Must match Langfuse's `SALT` env var (for API key hashing) |
| `LANGFUSE_ORG_ID` | Server env | The organization ID in our Langfuse instance |
| `LANGFUSE_BASE_URL` | Server env (default: `https://langfuse.astropods.ai`) | Langfuse instance URL |
| `LANGFUSE_AUTH_TOKEN` | Computed per-deploy, injected into collector | `base64(project_pk:project_sk)` for OTLP auth |
| `LANGFUSE_OTLP_ENDPOINT` | Collector env (optional override) | Override OTLP endpoint in collector config |

## 10. Key decisions

**Direct DB provisioning** — Langfuse's project/API-key management API requires an enterprise license we don't have. Since we self-host and control the database, we write directly to Langfuse's Postgres. The schema is Prisma-managed and stable. The key generation and hashing logic is simple: UUIDs with `pk-lf-`/`sk-lf-` prefixes, bcrypt + SHA256 hashing.

**Per-account project isolation** — Each Astro account gets its own Langfuse project. Traces are tenant-isolated by project, not by tags. Each account has its own API keys and its own view in the Langfuse UI.

**Lazy provisioning** — Projects created on first deploy, not on account creation. Idempotent — if `account_langfuse` row exists, skip. Avoids orphan projects.

**Dual secret storage** — The raw secret key is KMS-encrypted in astro's `account_langfuse` table (for runtime use). The hashed versions live only in Langfuse's `api_keys` table (for Langfuse's auth). This avoids needing to reverse hashes.

**Two database connections** — The server connects to both astro's Postgres and Langfuse's Postgres. The Langfuse connection is only used by the provisioner (writes) and is idle otherwise. Reads go through Langfuse's REST API using the per-account project keys.

**Dual-write, not switch** — Both backends receive the same traces. Galileo uses server-wide credentials. Langfuse uses per-account credentials. Compare side by side with no trace gaps.

**Fail-open** — If provisioning fails or Langfuse DB is unreachable, the deploy continues without Langfuse. The collector's Langfuse exporter errors silently. Galileo is unaffected.

**SALT dependency** — The astro server must know Langfuse's `SALT` env var to compute `fast_hashed_secret_key`. This is a coupling point — if the Langfuse SALT changes, existing keys break. In practice the SALT never changes on a running instance.
