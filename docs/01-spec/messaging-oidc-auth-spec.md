# Messaging Ingress Authentication Specification

> **Superseded.** The per-deployment ALB-OIDC mechanism below shipped, then
> was replaced by a front-door ALB listener rule under the tenant-router
> model — OIDC is now enforced once at the front door, not per deployment.
> None of the `MESSAGING_OIDC_*` env vars or the per-deployment
> `messaging-oidc` Secret this spec describes exist anymore
> (`apps/astro-server/internal/k8s/spec_applier.go` has the current comment
> and behavior; `modules/astro-infra/docs/architecture/14-tenant-router.md`
> is the as-built doc for the replacement). Kept here as the original design,
> not as current documentation.

**Version**: 2.0
**Status**: Implemented (superseded — see banner above)
**Author**: Astro Team
**Date**: 2026-04-07

## Overview

The messaging container's web adapter is exposed via an AWS ALB ingress. This spec covers adding authentication in front of the messaging ingress using ALB's native OIDC support, with no changes to the container.

Authentication is opt-in per deployment via the deployment template. Server-level env vars provide the OIDC credentials; each deployment explicitly enables auth via `interfaces.auth.web.type: oidc`.

## Goals

1. **Zero container changes**: Auth is enforced entirely at the ALB layer.
2. **WorkOS as IdP**: Reuse the existing WorkOS identity provider via its AuthKit OIDC endpoints.
3. **Explicit opt-in per deployment**: Auth is disabled by default. Each deployment template controls whether it is enabled.
4. **Session management**: ALB manages session cookies and token refresh.
5. **User-friendly toggle**: The deploy form exposes "Enable authentication" without surfacing OIDC/WorkOS terminology.

## Non-Goals

1. **Machine-to-machine auth**: ALB OIDC uses the browser Authorization Code flow; API clients are out of scope.
2. **Fine-grained authorization**: ALB only validates identity (authn). Per-user access control is handled separately (see Future Work).
3. **Per-deployment OIDC credentials**: A single server-level OIDC config applies to all deployments. Per-deployment credential override is reserved for `oidc-custom` (see Future Work).

## Design

### How ALB OIDC works

ALB's built-in authenticate-oidc action intercepts unauthenticated requests on the listener rule. It redirects the browser to the OIDC IdP, handles the callback at the fixed path `/oauth2/idpresponse`, exchanges the authorization code for tokens, validates them, sets an `AWSELBAuthSessionCookie`, then forwards the request to the container with identity headers:

- `x-amzn-oidc-identity` — subject claim (WorkOS user ID)
- `x-amzn-oidc-data` — signed JWT with user claims
- `x-amzn-oidc-accesstoken` — access token

The ALB controller reads OIDC client credentials from a Kubernetes secret (`messaging-oidc`) created in the agent namespace on every deploy.

### WorkOS OIDC endpoints

AuthKit domain: `https://<env>.authkit.app`

Discovery document: `https://<env>.authkit.app/.well-known/openid-configuration`

| Field | Value |
|-------|-------|
| Issuer | `https://<env>.authkit.app` |
| Authorization endpoint | `https://<env>.authkit.app/oauth2/authorize` |
| Token endpoint | `https://<env>.authkit.app/oauth2/token` |
| UserInfo endpoint | `https://<env>.authkit.app/oauth2/userinfo` |

### Redirect URI

ALB's fixed callback path is `/oauth2/idpresponse`. Register a wildcard redirect URI in the WorkOS OIDC application:

```
https://*.<ingress-domain>/oauth2/idpresponse
```

### K8s secret

The ALB controller reads OIDC credentials from a Kubernetes secret in the agent namespace. The server creates this secret on every deploy when auth is enabled:

```
name: messaging-oidc
namespace: <agent-namespace>
data:
  clientId: <base64>
  clientSecret: <base64>
```

The ALB controller's ClusterRole must include `get`/`list`/`watch` on `secrets`.

## Implementation

### Deployment spec — `packages/astro-spec/deployment_spec.go`

`DeploymentInterfaces` has an optional `Auth` field:

```go
type DeploymentInterfaces struct {
    // ... existing fields ...
    Auth *DeploymentInterfacesAuth `json:"auth,omitempty"`
}

type DeploymentInterfacesAuth struct {
    Web *DeploymentWebAuth `json:"web,omitempty"`
}

type DeploymentWebAuth struct {
    Type string `json:"type,omitempty"` // "oidc" | "oidc-custom"
}
```

### Server config — `apps/astro-server/internal/config/config.go`

| Env var | Purpose |
|---------|---------|
| `MESSAGING_OIDC_ISSUER` | WorkOS AuthKit issuer URL |
| `MESSAGING_OIDC_AUTH_ENDPOINT` | Authorization endpoint |
| `MESSAGING_OIDC_TOKEN_ENDPOINT` | Token endpoint |
| `MESSAGING_OIDC_USERINFO_ENDPOINT` | UserInfo endpoint |
| `MESSAGING_OIDC_CLIENT_ID` | OIDC application client ID |
| `MESSAGING_OIDC_CLIENT_SECRET` | OIDC application client secret |
| `MESSAGING_OIDC_SESSION_TIMEOUT` | Session duration in seconds (default 3600) |

All vars are optional. If `MESSAGING_OIDC_ISSUER` is unset, auth is fully disabled regardless of deployment spec.

### Deployment flow — `apps/astro-server/internal/k8s/spec_applier.go`

On deploy, when `ds.Interfaces.Auth.Web.Type == "oidc"` AND server OIDC is configured:

1. Create/update the `messaging-oidc` Kubernetes secret in the agent namespace
2. Build the ingress with 5 ALB auth annotations:
   - `alb.ingress.kubernetes.io/auth-type: oidc`
   - `alb.ingress.kubernetes.io/auth-idp-oidc` — JSON with issuer/endpoints/secretName
   - `alb.ingress.kubernetes.io/auth-on-unauthenticated-request: authenticate`
   - `alb.ingress.kubernetes.io/auth-scope: openid email`
   - `alb.ingress.kubernetes.io/auth-session-timeout: <seconds>`

If `auth` is absent or `web` is nil, no auth annotations are added.

### Frontend — `apps/astro-client`

The web adapter card in the messaging interfaces picker shows an "Enable authentication" toggle when selected. When enabled, `interfaces.auth.web.type: oidc` is set in the fulfilled deployment spec. Pre-fills correctly when editing an existing deployment.

## Infra requirements

```hcl
resource "aws_secretsmanager_secret" "messaging_oidc" {
  name = "preview-messaging-oidc"
}

resource "aws_secretsmanager_secret_version" "messaging_oidc" {
  secret_id     = aws_secretsmanager_secret.messaging_oidc.id
  secret_string = jsonencode({
    clientId     = var.workos_messaging_oidc_client_id
    clientSecret = var.workos_messaging_oidc_client_secret
  })
}
```

The server reads `MESSAGING_OIDC_CLIENT_ID` and `MESSAGING_OIDC_CLIENT_SECRET` from the deployment secrets (not from Secrets Manager directly — Secrets Manager is used only to store them securely in infra).

The ALB controller's IAM role needs `secretsmanager:GetSecretValue` on the secret ARN — but note this is only needed if the ALB controller fetches from Secrets Manager directly. In the current implementation, the server creates a K8s secret and the ALB controller reads from K8s.

## Migration

No action required for existing deployments. Auth is only applied when:
1. `MESSAGING_OIDC_ISSUER` (and client credentials) are set on the server, AND
2. The deployment spec has `interfaces.auth.web.type: oidc`

Existing deployments without `auth` in their spec will not get auth applied on redeploy.

## Future Work

### Per-user access control

ALB injects `x-amzn-oidc-identity` (WorkOS user ID) on every authenticated request. The messaging container's web adapter supports `WEB_ALLOWED_USER_IDS` (comma-separated) to restrict access to specific users without any infra changes.

### Per-deployment OIDC credentials (`oidc-custom`)

`DeploymentWebAuth.Type: "oidc-custom"` is reserved for future per-deployment credential override. This would allow different WorkOS applications per agent.
