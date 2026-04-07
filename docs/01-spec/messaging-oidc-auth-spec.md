# Messaging Ingress OIDC Authentication Specification

**Version**: 1.0
**Status**: Draft
**Author**: Astro Team
**Date**: 2026-04-07

## Overview

The messaging container's web adapter is exposed via an AWS ALB ingress with no authentication. This spec covers adding WorkOS OIDC authentication in front of the messaging ingress using ALB's native OIDC support, with no changes to the container.

## Goals

1. **Zero container changes**: Auth is enforced entirely at the ALB layer.
2. **WorkOS as IdP**: Reuse the existing WorkOS identity provider via its OIDC endpoints.
3. **Opt-in per deployment**: OIDC auth is configured per-ingress via server env vars; absent config means no auth (preserves backward compat).
4. **Session management**: ALB manages session cookies and token refresh.

## Non-Goals

1. **Machine-to-machine auth**: ALB OIDC uses the browser Authorization Code flow; API clients are out of scope.
2. **Fine-grained authorization**: ALB only validates identity (authn). Authz (what the user can do) remains the agent's responsibility.
3. **Per-agent OIDC config**: Single global OIDC config applies to all messaging ingresses (see Future Work).

## Design

### How ALB OIDC works

ALB's built-in authenticate-oidc action intercepts unauthenticated requests on the listener rule. It redirects the browser to the OIDC IdP, handles the callback at the fixed path `/oauth2/idpresponse`, exchanges the authorization code for tokens, validates them, sets an `AWSELBAuthSessionCookie`, then forwards the request to the target with identity headers injected:

- `x-amzn-oidc-identity` — subject claim
- `x-amzn-oidc-data` — signed JWT with user claims
- `x-amzn-oidc-accesstoken` — access token

ALB retrieves the client secret from AWS Secrets Manager at request time using the ALB controller's IAM role.

### WorkOS OIDC endpoints

WorkOS exposes standard OIDC endpoints. The discovery document is at `https://api.workos.com/.well-known/openid-configuration`. ALB requires explicit endpoint URLs (no auto-discovery):

| Field | Value |
|-------|-------|
| Issuer | from discovery doc |
| Authorization endpoint | from discovery doc |
| Token endpoint | from discovery doc |
| UserInfo endpoint | from discovery doc |

### Redirect URI

ALB's fixed callback path is `/oauth2/idpresponse`. The redirect URI registered with WorkOS must be `https://<messaging-host>/oauth2/idpresponse`.

Since each agent gets a unique host (via `GenerateIngressHost`), a wildcard redirect URI must be registered in WorkOS, or each agent's host must be registered individually. Wildcard is preferred for operational simplicity.

### Secrets Manager secret format

```json
{ "clientId": "<workos-client-id>", "clientSecret": "<workos-client-secret>" }
```

The ALB controller's service account (via IRSA) needs `secretsmanager:GetSecretValue` on this ARN.

## Implementation

### `IngressConfig` — new optional OIDC fields (`ingress.go`)

Add an `OIDCAuthConfig` struct:

- `Issuer` string
- `AuthorizationEndpoint` string
- `TokenEndpoint` string
- `UserInfoEndpoint` string
- `SecretsManagerARN` string — ARN of the Secrets Manager secret
- `Scope` string — default `"openid email"`
- `SessionTimeoutSeconds` int — default `3600`

Add `OIDCAuth *OIDCAuthConfig` to `IngressConfig`. Nil means no auth annotations are added.

`BuildIngress` adds these ALB annotations when `OIDCAuth` is non-nil:

- `alb.ingress.kubernetes.io/auth-type: oidc`
- `alb.ingress.kubernetes.io/auth-idp-oidc` — JSON-encoded struct with `issuer`, `authorizationEndpoint`, `tokenEndpoint`, `userInfoEndpoint`, `secretName` (the ARN)
- `alb.ingress.kubernetes.io/auth-on-unauthenticated-request: authenticate`
- `alb.ingress.kubernetes.io/auth-scope` — from config
- `alb.ingress.kubernetes.io/auth-session-timeout` — from config

### `spec_applier.go` — pass OIDC config

When building the messaging ingress, populate `IngressConfig.OIDCAuth` from the applier's config. If any required field is empty, log a warning and skip OIDC (fail open, not closed, to avoid blocking deployments during misconfiguration).

### Server config — new env vars

| Env var | Purpose |
|---------|---------|
| `MESSAGING_OIDC_ISSUER` | WorkOS issuer URL |
| `MESSAGING_OIDC_AUTH_ENDPOINT` | Authorization endpoint |
| `MESSAGING_OIDC_TOKEN_ENDPOINT` | Token endpoint |
| `MESSAGING_OIDC_USERINFO_ENDPOINT` | UserInfo endpoint |
| `MESSAGING_OIDC_SECRET_ARN` | Secrets Manager ARN for client credentials |
| `MESSAGING_OIDC_SESSION_TIMEOUT` | Session duration in seconds (default 3600) |

All vars are optional. If none are set, OIDC is disabled and the ingress is created as-is today.

## Future Work

### Per-deployment OIDC override

The global env var config applies uniformly to all messaging ingresses. A future extension would allow operators to override auth config per deployment via `DeploymentInterfaces.Auth` in `AstroDeploymentSpec` (`packages/astro-spec/deployment_spec.go`):

```go
type DeploymentInterfaces struct {
    // ... existing fields ...
    Auth *MessagingAuthConfig `json:"auth,omitempty"`
}

type MessagingAuthConfig struct {
    Type              string // "oidc" | "none"
    Issuer            string
    AuthEndpoint      string
    TokenEndpoint     string
    UserInfoEndpoint  string
    SecretsManagerARN string
    SessionTimeout    int
}
```

Override semantics:
- `auth` absent → inherit server global config
- `auth.type: none` → explicitly disable auth for this deployment
- `auth.type: oidc` with fields → override server defaults for this deployment

This is purely a server-side deployment spec concern. It does not belong in `astropods.yml` (agent author concern) — it is an operator-level decision set at deploy time.

## Migration

No changes required for existing deployments. OIDC auth is activated by setting the env vars above on the server. On next deploy of an agent, the messaging ingress will be updated with auth annotations.

Agents deployed before the env vars are set will continue to work without auth until they are redeployed (the ingress is only updated on deploy).
