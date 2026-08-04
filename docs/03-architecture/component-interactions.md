# Astro Component Architecture

This document describes the architecture and interactions between the three core Astro components: **astro-cli**, **astro-registry**, and **astro-server**.

> **Note:** Registry namespaces are **account names** (the active account, set via `ast account switch`), not per-user `user_id`s. The registry resolves a namespace to an account, authorizes by **membership** (plus `agents:read` / `agents:write` for org accounts), and rewrites to the ECR repository `{env}-tenant-{account_id}`. The `user123` in the diagrams below is an illustrative account name.

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
- `cmd/push.go`, `cmd/push_streaming.go` - the `push` command (build, push, register)
- `internal/auth/` - Token management and device auth flow
- `internal/auth/crane.go` - Image push via crane library

### astro-registry

**Purpose**: Authenticated Docker Registry V2 proxy fronting AWS ECR.

**Responsibilities**:
- Validate JWT tokens via WorkOS JWKS endpoint
- Enforce account-scoped access control (a user can only pull/push within accounts they are a member of)
- Proxy Docker Registry V2 API calls to ECR
- Add an `{env}-tenant-{account}` prefix to namespaces when proxying to ECR (transparent to CLI)
- Manage ECR auth tokens with automatic refresh (IRSA in K8s)
- Issue registry-scoped tokens at `GET /token` (there is no `/api/namespace`; the namespace is the active account)

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
- `handlers/agents.go` - Agent registration/listing
- `handlers/deploy.go` - Deployment orchestration
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
Developer runs: ast push <name>

1. CLI: Load and parse astropods.yml
2. CLI: Build images locally (docker build)
3. CLI: Tag images under the active-account namespace
   └─ registry.example.com/{account}/{agent_name}:{tag}
4. CLI: Push images via crane
   └─ Registry authorizes by account membership, adds the {env}-tenant-{account} prefix, proxies to ECR
5. CLI: Transform spec (build: → image:)
6. CLI: POST /api/v1/agents/{account}/{name}/register to server
   └─ Server stores spec in agent index
```

## ECR Tenant Namespace Mapping

The registry transparently maps account namespaces to ECR repositories with an `{env}-tenant-{account_id}` prefix. This is required to comply with ECR IAM policy restrictions. (The `user123` in the diagram below stands in for the account namespace.)

```
CLI                           Registry Proxy                    ECR
 │                                 │                              │
 │  Push: /v2/user123/mybot/...   │                              │
 │ ───────────────────────────────>│                              │
 │                                 │                              │
 │                                 │  1. Authz: caller is a member of account user123 ✓
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

Access is enforced when the registry issues a scoped token at `GET /token`. The namespace is an **account name**; the registry resolves it to an account and checks the caller's membership (`IsMemberWithID`):

| Action           | Rule                                                                          |
|------------------|-------------------------------------------------------------------------------|
| `pull`           | Caller must be a member of the account; org accounts also require `agents:read`.  |
| `push` / `delete`| Caller must be a member of the account; org accounts also require `agents:write`. |

Personal accounts (no org context on the JWT) require membership only — no permission check. The resolved account id drives the `{env}-tenant-{account_id}` ECR rewrite.

**Namespace extraction from path**:
- `/v2/{namespace}/{image}/manifests/{tag}` → namespace = first path segment after `/v2/`

## API Contracts

### Registry: Token

The registry has no `/api/namespace`. The push namespace is the active account (set via `ast account switch`); Docker's login/pull obtains a registry-scoped token from `GET /token`. See [registry-token-auth.md](registry-token-auth.md) for the token contract.

### Server: Register Agent

```
POST /api/v1/agents/{account}/{name}/register
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "my-agent",
  "version": "1.0.0",
  "registry": "registry.example.com/{account}",
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
| Agent | `{registry}/{account}/{agent_name}:{tag}` |
| Model | `{registry}/{account}/{agent_name}-model-{name}:{tag}` |
| Knowledge | `{registry}/{account}/{agent_name}-knowledge-{name}:{tag}` |
| Tool | `{registry}/{account}/{agent_name}-tool-{name}:{tag}` |
| Interface | `{registry}/{account}/{agent_name}-interface-{name}:{tag}` |

**ECR perspective** (actual storage, managed by registry):

| Type | Pattern |
|------|---------|
| Agent | `{ecr}/{env}-tenant-{account_id}/{agent_name}:{tag}` |
| Model | `{ecr}/{env}-tenant-{account_id}/{agent_name}-model-{name}:{tag}` |
| Knowledge | `{ecr}/{env}-tenant-{account_id}/{agent_name}-knowledge-{name}:{tag}` |
| Tool | `{ecr}/{env}-tenant-{account_id}/{agent_name}-tool-{name}:{tag}` |
| Interface | `{ecr}/{env}-tenant-{account_id}/{agent_name}-interface-{name}:{tag}` |

## Token Storage

| Context | Storage | Notes |
|---------|---------|-------|
| CLI | System keyring or `~/.ast/credentials.json` | Keyring preferred, file fallback |
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
