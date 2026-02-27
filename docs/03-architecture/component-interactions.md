# Astro Component Architecture

This document describes the architecture and interactions between the three core Astro components: **astro-cli**, **astro-registry**, and **astro-server**.

## Component Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Developer Workflow                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              astro-cli                                       │
│  • Authenticates via WorkOS device flow                                     │
│  • Builds container images locally                                          │
│  • Pushes images to registry                                                │
│  • Registers agent specs with server                                        │
└─────────────────────────────────────────────────────────────────────────────┘
                    │                                   │
                    │ Push images                       │ Register specs
                    ▼                                   ▼
┌────────────────────────────────┐     ┌────────────────────────────────────┐
│       astro-registry           │     │          astro-server              │
│                                │     │                                    │
│  • Docker Registry V2 proxy    │     │  • Agent catalog/index             │
│  • JWT validation via JWKS     │     │  • Spec validation                 │
│  • Namespace access control    │     │  • Kubernetes deployment           │
│  • ECR backend proxy           │     │  • Web auth (WorkOS AuthKit)       │
└────────────────────────────────┘     └────────────────────────────────────┘
                    │                                   │
                    ▼                                   ▼
            ┌──────────────┐                   ┌──────────────────┐
            │   AWS ECR    │                   │   EKS Cluster    │
            └──────────────┘                   └──────────────────┘
```

## Component Details

### astro-cli

**Purpose**: Developer tool for building and publishing agents.

**Responsibilities**:
- Authenticate users via WorkOS device authorization flow
- Build container images from `astropods.yml` spec
- Push images to astro-registry with bearer auth
- Transform specs (replace `build:` with `image:` refs)
- Register transformed specs with astro-server

**Key Files**:
- `cmd/publish.go` - Main publish command
- `internal/auth/` - Token management and device auth flow
- `internal/auth/crane.go` - Image push via crane library

### astro-registry

**Purpose**: Authenticated Docker Registry V2 proxy fronting AWS ECR.

**Responsibilities**:
- Validate JWT tokens via WorkOS JWKS endpoint
- Enforce namespace-based access control (user can only push to their namespace)
- Proxy Docker Registry V2 API calls to ECR
- Add `tenant-` prefix to namespaces when proxying to ECR (transparent to CLI)
- Manage ECR auth tokens with automatic refresh (IRSA in K8s)
- Provide `/api/namespace` endpoint for CLI to discover user's namespace

**Key Files**:
- `handlers/registry_proxy.go` - Registry V2 proxy implementation
- `handlers/probes.go` - K8s health probes (livez, readyz, healthz)
- `internal/auth/jwt.go` - JWT validation
- `internal/registry/ecr.go` - ECR auth token management
- `internal/middleware/auth.go` - Auth middleware

### astro-server

**Purpose**: Central deployment orchestration and agent catalog.

**Responsibilities**:
- Host WorkOS OAuth via AuthKit for web clients
- Store and index agent specs in database
- Validate agent specs
- Translate specs to Kubernetes manifests
- Deploy/undeploy to EKS clusters

**Key Files**:
- `internal/handlers/agents.go` - Agent registration/listing
- `internal/handlers/deploy.go` - Deployment orchestration
- `internal/middleware/auth.go` - API auth middleware

## Authentication Flow

### CLI Authentication (Device Flow)

```
astro-cli                         WorkOS
    │                               │
    ├─ POST /user_management/authorize/device
    │  ────────────────────────────>│
    │  <────────────────────────────┤
    │  { device_code, user_code, verification_uri }
    │                               │
    ├─ Open browser to verification_uri
    │  User enters code and authenticates
    │                               │
    ├─ Poll POST /user_management/authenticate
    │  ────────────────────────────>│
    │  <────────────────────────────┤
    │  { access_token, refresh_token }
    │                               │
    └─ Store tokens in keyring/file
```

### Token Validation (Registry & Server)

Both components validate WorkOS JWTs using JWKS:

1. Extract `Bearer <token>` from Authorization header
2. Fetch JWKS from WorkOS endpoint (cached 1 hour)
3. Validate signature using RS256 public key
4. Extract claims: `sub` (user_id), `org_id`, `sid` (session_id)
5. Set user/session context for downstream handlers

## Image Publishing Flow

```
Developer runs: astro publish --tag v1.0.0

1. CLI: Load and parse astropods.yml
2. CLI: GET /api/namespace → registry validates token, returns user_id
3. CLI: Build images locally (docker build)
4. CLI: Tag images with registry namespace
   └─ registry.example.com/{user_id}/{agent_name}:{tag}
5. CLI: Push images via crane
   └─ Registry validates namespace, adds tenant- prefix, proxies to ECR
6. CLI: Transform spec (build: → image:)
7. CLI: POST /api/v1/agents/register to server
   └─ Server stores spec in agent index
```

## ECR Tenant Namespace Mapping

The registry transparently maps user namespaces to ECR repositories with a `tenant-` prefix. This is required to comply with ECR IAM policy restrictions.

```
CLI                           Registry Proxy                    ECR
 │                                 │                              │
 │  Push: /v2/user123/mybot/...   │                              │
 │ ───────────────────────────────>│                              │
 │                                 │                              │
 │                                 │  1. Validate: user123 == user.ID ✓
 │                                 │  2. Rewrite: add tenant- prefix
 │                                 │                              │
 │                                 │  Push: /v2/tenant-user123/mybot/...
 │                                 │ ─────────────────────────────>│
 │                                 │                              │
 │                                 │  Location: .../tenant-user123/...
 │                                 │ <─────────────────────────────│
 │                                 │                              │
 │                                 │  3. Rewrite: strip tenant- prefix
 │                                 │                              │
 │  Location: .../user123/...     │                              │
 │ <───────────────────────────────│                              │
```

**Key points:**
- CLI is unaware of the `tenant-` prefix
- Registry adds `tenant-` when proxying requests to ECR
- Registry strips `tenant-` from Location headers in responses
- ECR IAM policy allows `arn:aws:ecr:*:*:repository/tenant-*`
- ECR auth uses IRSA (IAM Roles for Service Accounts) in K8s

## Namespace Access Control

The registry enforces namespace-based access:

| Operation | Rule |
|-----------|------|
| Read (GET/HEAD) | Any authenticated user |
| Write (PUT/POST/PATCH/DELETE) | Only if namespace = user_id OR org_id |

**Namespace extraction from path**:
- `/v2/{namespace}/{image}/manifests/{tag}` → namespace = first path segment after `/v2/`

## API Contracts

### Registry: Get Namespace

```
GET /api/namespace
Authorization: Bearer <token>

Response 200:
{
  "user_id": "user_123",
  "organization_id": "org_456"  // optional
}
```

### Server: Register Agent

```
POST /api/v1/agents/register
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "my-agent",
  "version": "1.0.0",
  "registry": "registry.example.com/user_123",
  "spec_content": "<YAML with image refs>"
}

Response 201:
{
  "message": "Agent registered successfully",
  "name": "my-agent",
  "version": "1.0.0"
}
```

### Server: Deploy Agent

```
POST /api/v1/deploy
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "my-agent",
  "version": "1.0.0",
  "k8s_namespace": "astro-agents",
  "user_credentials": {
    "OPENAI_API_KEY": "sk-..."
  }
}

Response 200:
{
  "status": "success",
  "deployed_at": "2026-02-02T10:35:00Z",
  "resources": [...],
  "service_endpoints": [...]
}
```

## Image Naming Convention

**CLI perspective** (what developers see):

| Type | Pattern |
|------|---------|
| Agent | `{registry}/{user_id}/{agent_name}:{tag}` |
| Model | `{registry}/{user_id}/{agent_name}-model-{name}:{tag}` |
| Knowledge | `{registry}/{user_id}/{agent_name}-knowledge-{name}:{tag}` |
| Tool | `{registry}/{user_id}/{agent_name}-tool-{name}:{tag}` |
| Interface | `{registry}/{user_id}/{agent_name}-interface-{name}:{tag}` |

**ECR perspective** (actual storage, managed by registry):

| Type | Pattern |
|------|---------|
| Agent | `{ecr}/tenant-{user_id}/{agent_name}:{tag}` |
| Model | `{ecr}/tenant-{user_id}/{agent_name}-model-{name}:{tag}` |
| Knowledge | `{ecr}/tenant-{user_id}/{agent_name}-knowledge-{name}:{tag}` |
| Tool | `{ecr}/tenant-{user_id}/{agent_name}-tool-{name}:{tag}` |
| Interface | `{ecr}/tenant-{user_id}/{agent_name}-interface-{name}:{tag}` |

## Token Storage

| Context | Storage | Notes |
|---------|---------|-------|
| CLI | System keyring or `~/.astro/credentials.json` | Keyring preferred, file fallback |
| Server (web) | Encrypted session cookie | httpOnly, SameSite |
| CI/CD | `ASTRO_ACCESS_TOKEN` env var | Override for automation |

## Security Notes

- All tokens are WorkOS JWTs validated via JWKS (RS256)
- Registry proxies ECR auth internally (users never see ECR tokens)
- ECR auth uses IRSA (IAM Roles for Service Accounts) - no static credentials
- ECR IAM policy restricts access to `tenant-*` repositories only
- Session cookies are sealed (encrypted + integrity protected)
- Write operations require namespace ownership verification
- HTTPS required in production
